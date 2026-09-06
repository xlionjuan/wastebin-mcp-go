package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"

	"wastebin-mcp-go/internal/wastebin"
)

// Process exit codes.
const (
	exitCodeSuccess  = 0
	exitCodeCLIError = 1
	exitCodeMCPError = 2
)

// errContentEmptyCLI is returned when --content flag is explicitly set to empty.
var errContentEmptyCLI = errors.New("--content must not be empty")

// errUnexpectedArgs is returned when positional arguments are provided to a
// subcommand that does not accept them.
var errUnexpectedArgs = errors.New("unexpected arguments")

// errMissingContentOrFile is returned when neither --content nor --file-path
// is provided to the create subcommand.
var errMissingContentOrFile = errors.New("either --content or --file-path must be provided")

var (
	version = "v0.13.1"
	commit  = "none"
	date    = "unknown"
)

// CLIFlags holds parsed CLI create-command flag values.
type CLIFlags struct {
	Content          string
	FilePath         string
	Extension        string
	Expires          string
	Title            string
	BurnAfterReading bool
	Password         string
	PasswordSet      bool
	Debug            bool
	Help             bool
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI parses CLI arguments, executes the requested action, and returns an
// exit code. It is a testable entry point that does not call os.Exit.
func runCLI(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		cfg, err := wastebin.ConfigFromEnv()
		if err != nil {
			//nolint:errcheck // best-effort write to stderr
			fmt.Fprintf(stderr, "ERROR: %v\n", err)

			return exitCodeMCPError
		}

		if cfg.Debug {
			slog.SetLogLoggerLevel(slog.LevelDebug)
		}

		mcpStdin, err := prepareMCPStdin(os.Stdin, cfg)
		if err != nil {
			//nolint:errcheck // best-effort write to stderr
			fmt.Fprintf(stderr, "ERROR: %v\n", err)

			return exitCodeMCPError
		}

		err = runMCPMode(cfg, mcpStdin)
		if err != nil {
			slog.Error("MCP server error", "error", err)

			return exitCodeMCPError
		}

		return exitCodeSuccess
	}

	switch args[0] {
	case "create":
		err := runCreateCommand(args[1:], stdout)
		if err != nil {
			//nolint:errcheck // best-effort write to stderr
			fmt.Fprintf(stderr, "ERROR: %v\n\n", err)
			printCLIHelp(stderr)

			return exitCodeCLIError
		}
	case "--help":
		printCLIHelp(stdout)
	case "--version":
		//nolint:errcheck // best-effort write to stdout
		fmt.Fprintf(stdout, "wastebin-mcp-go version %s (commit: %s, built: %s)\n", version, commit, date)
	default:
		//nolint:errcheck // best-effort write to stderr
		fmt.Fprintf(stderr, "ERROR: unknown command or flag: %q\n\n", args[0])
		printCLIHelp(stderr)

		return exitCodeCLIError
	}

	return exitCodeSuccess
}

// parseCreateFlags parses CLI flags for the "create" subcommand using Go's flag
// package.
func parseCreateFlags(args []string) (*CLIFlags, error) {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	flags := &CLIFlags{}

	fs.StringVar(&flags.Content, "content", "", "Paste content")
	fs.StringVar(&flags.FilePath, "file-path", "", "Path to file to read")
	fs.StringVar(&flags.Extension, "extension", "", "Syntax highlighting extension")
	fs.StringVar(&flags.Expires, "expires", "", "Expiration duration")
	fs.StringVar(&flags.Title, "title", "", "Paste title")
	fs.BoolVar(&flags.BurnAfterReading, "burn-after-reading", false, "Delete after first read")
	fs.StringVar(&flags.Password, "password", "", "Encryption password")
	fs.BoolVar(&flags.Debug, "debug", false, "Enable debug logging")
	fs.BoolVar(&flags.Help, "help", false, "Show this help message")

	err := fs.Parse(args)
	if err != nil {
		return nil, err
	}

	// Reject positional arguments — create takes flags only.
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("%w: %v", errUnexpectedArgs, fs.Args())
	}

	// Detect if --content or --password were explicitly set.
	// Go's flag package cannot distinguish "not set" from "set to zero value"
	// via the value alone, but fs.Visit() only visits flags that were parsed.
	contentExplicitlySet := false

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "content":
			contentExplicitlySet = true
		case "password":
			flags.PasswordSet = true
		}
	})

	if contentExplicitlySet && flags.Content == "" {
		return nil, errContentEmptyCLI
	}

	return flags, nil
}

// runCreateCommand parses the create subcommand arguments, validates them, and
// executes the paste creation flow. It returns an error if validation fails or
// execution fails. The stdout writer is used for help-text output.
func runCreateCommand(args []string, stdout io.Writer) error {
	flags, err := parseCreateFlags(args)
	if err != nil {
		return err
	}

	if flags.Help {
		printCLIHelp(stdout)

		return nil
	}

	if flags.Content == "" && flags.FilePath == "" {
		return errMissingContentOrFile
	}

	return runCLIMode(flags)
}
