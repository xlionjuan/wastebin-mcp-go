package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"wastebin-mcp-go/internal/wastebin"
)

// printCLIHelp prints the help text showing the create subcommand usage, all
// flags, examples, and exit codes.
func printCLIHelp() {
	fmt.Print(`wastebin-mcp-go - MCP server + CLI for Wastebin pastebin

USAGE:
  wastebin-mcp-go create [OPTIONS]    Create a paste
  wastebin-mcp-go                     Start MCP stdio server

CREATE OPTIONS:
  --content TEXT         Paste content
  --file-path PATH       Read content from file
  --extension EXT        Syntax highlighting extension (e.g. md, go, py)
  --expires DURATION     Expiration: number=seconds, or with unit (s/m/h/d/w/M/y)
  --title TEXT           Paste title
  --burn-after-reading   Delete paste after first read
  --password TEXT        Encrypt paste with password
  --debug                Enable debug logging

GLOBAL OPTIONS:
  --help                 Show this help message
  --version              Show version information

EXIT CODES:
  0   Success
  1   CLI error / invalid arguments
  2   MCP server error

For more information, see: https://github.com/xlionjuan/wastebin-mcp-go
`)
}

// runCLIMode executes the one-shot paste creation flow: reads configuration
// from the environment, creates a Wastebin client, builds CreatePasteArgs from
// the provided CLI flags, calls CreatePaste, and prints the JSON response to
// stdout.
func runCLIMode(flags *CLIFlags) error {
	cfg, err := wastebin.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	if flags.Debug {
		cfg.Debug = true
	}

	if cfg.Debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
	}

	client, err := wastebin.NewWastebinClient(cfg)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	args := buildCreatePasteArgs(flags)

	ctx := context.Background()

	resp, err := client.CreatePaste(ctx, args)
	if err != nil {
		return fmt.Errorf("create paste failed: %w", err)
	}

	err = json.NewEncoder(os.Stdout).Encode(resp)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return nil
}

// buildCreatePasteArgs converts CLIFlags into a CreatePasteArgs, setting only
// the fields that were explicitly provided.
func buildCreatePasteArgs(flags *CLIFlags) *wastebin.CreatePasteArgs {
	args := &wastebin.CreatePasteArgs{}

	if flags.Content != "" {
		args.Content = &flags.Content
	}

	if flags.FilePath != "" {
		args.FilePath = &flags.FilePath
	}

	if flags.Extension != "" {
		args.Extension = &flags.Extension
	}

	if flags.Expires != "" {
		args.Expires = &flags.Expires
	}

	if flags.Title != "" {
		args.Title = &flags.Title
	}

	if flags.BurnAfterReading {
		args.BurnAfterReading = &flags.BurnAfterReading
	}

	if flags.Password != "" {
		args.Password = &flags.Password
	}

	return args
}
