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

var (
	version = "v0.9.0"
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
	Debug            bool
	Help             bool
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		runMCPModeFromArgs()

		return
	}

	switch args[0] {
	case "create":
		flags, err := parseCreateFlags(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n\n", err)
			printCLIHelp()
			os.Exit(exitCodeCLIError)
		}

		if flags.Help {
			printCLIHelp()

			return
		}

		err = runCLIMode(flags)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
			os.Exit(exitCodeCLIError)
		}
	case "--help":
		printCLIHelp()
	case "--version":
		fmt.Printf("wastebin-mcp-go version %s (commit: %s, built: %s)\n", version, commit, date)
	default:
		fmt.Fprintf(os.Stderr, "ERROR: unknown command or flag: %q\n\n", args[0])
		printCLIHelp()
		os.Exit(exitCodeCLIError)
	}
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

	// Detect if --content was explicitly set to empty string.
	// Go's flag package cannot distinguish "not set" from "set to zero value"
	// via the value alone, but fs.Visit() only visits flags that were parsed.
	contentExplicitlySet := false

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "content" {
			contentExplicitlySet = true
		}
	})

	if contentExplicitlySet && flags.Content == "" {
		return nil, errContentEmptyCLI
	}

	return flags, nil
}

// runMCPModeFromArgs reads configuration from environment variables, validates
// that stdin contains a valid MCP initialize message, and starts the MCP stdio
// server. It exits the process with exitCodeMCPError on failure.
func runMCPModeFromArgs() {
	cfg, err := wastebin.ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(exitCodeMCPError)
	}

	if cfg.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	mcpStdin, err := prepareMCPStdin(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(exitCodeMCPError)
	}

	err = runMCPMode(cfg, mcpStdin)
	if err != nil {
		slog.Error("MCP server error", "error", err)
		os.Exit(exitCodeMCPError)
	}
}
