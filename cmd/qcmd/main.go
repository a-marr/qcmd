// Package main is the entry point for the qcmd CLI tool.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/user/qcmd/internal/backend"
	"github.com/user/qcmd/internal/config"
	"github.com/user/qcmd/internal/editor"
	"github.com/user/qcmd/internal/output"
	"github.com/user/qcmd/internal/safety"
	"github.com/user/qcmd/internal/sanitize"
	"github.com/user/qcmd/internal/shellctx"
)

// Exit codes following the project specification.
const (
	exitSuccess        = 0
	exitUserError      = 1
	exitSystemError    = 2
	exitDangerBlocked  = 3
	maxQueryLength     = 10000
)

// version is set at build time via ldflags: -X main.version=...
var version = "dev"

// flags holds all command-line flags.
type flags struct {
	queryFile  string
	query      string
	backendStr string
	model      string
	outputMode string
	noSafety   bool
	configPath string
	verbose    bool
	showVer    bool
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// Check for subcommands first (before flag parsing).
	if len(args) > 0 {
		switch args[0] {
		case "config":
			return handleConfigCommand(args[1:])
		case "backends":
			return handleBackendsCommand()
		}
	}

	// Parse flags.
	f, err := parseFlags(args)
	if err != nil {
		// If it's a help request, flag package already printed help.
		if errors.Is(err, flag.ErrHelp) {
			return exitSuccess
		}
		fmt.Fprintf(os.Stderr, "qcmd: %v\n", err)
		return exitUserError
	}

	// Handle --version.
	if f.showVer {
		fmt.Printf("qcmd version %s\n", version)
		return exitSuccess
	}

	// Load configuration.
	cfg, err := config.Load(&config.LoadOptions{ConfigPath: f.configPath})
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: failed to load config: %v\n", err)
		return exitSystemError
	}

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: invalid config: %v\n", err)
		return exitUserError
	}

	// Override backend from flag if provided.
	backendName := cfg.Backend
	if f.backendStr != "" {
		backendName = f.backendStr
	}

	// Override model from flag if provided.
	modelName := cfg.GetModel(backendName)
	if f.model != "" {
		modelName = f.model
	}

	// Parse output mode.
	outputMode, err := output.ParseMode(f.outputMode)
	if err != nil {
		// If flag is provided but invalid, use config default.
		if f.outputMode == "" {
			outputMode, _ = output.ParseMode(cfg.OutputMode)
		} else {
			fmt.Fprintf(os.Stderr, "qcmd: invalid output mode: %s\n", f.outputMode)
			return exitUserError
		}
	}

	// Get query input.
	query, err := getQuery(f, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: %v\n", err)
		return exitUserError
	}

	// Validate input.
	if err := validateInput(query); err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: %v\n", err)
		return exitUserError
	}

	// Warn if both --query and --query-file are provided.
	if f.verbose && f.queryFile != "" && f.query != "" {
		fmt.Fprintln(os.Stderr, "qcmd: warning: --query-file takes precedence over --query")
	}

	// Create backend.
	be, err := createBackend(backendName, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: %v\n", err)
		return exitUserError
	}

	// Gather shell context if enabled.
	var shellContext *backend.ShellContext
	if cfg.IncludeContext {
		shellContext = shellctx.GatherContext()
	}

	// Create context with timeout.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()

	// Build request.
	req := &backend.Request{
		Query:   query,
		Context: shellContext,
		Model:   modelName,
	}

	if f.verbose {
		fmt.Fprintf(os.Stderr, "qcmd: using backend=%s model=%s\n", backendName, modelName)
	}

	// Call LLM backend.
	resp, err := be.GenerateCommand(ctx, req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Fprintln(os.Stderr, "qcmd: request timed out")
			return exitSystemError
		}
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, "qcmd: request canceled")
			return exitSystemError
		}
		if errors.Is(err, backend.ErrNoAPIKey) {
			fmt.Fprintf(os.Stderr, "qcmd: no API key configured for backend %q\n", backendName)
			fmt.Fprintf(os.Stderr, "  Set %s_API_KEY environment variable or add api_key to config\n", strings.ToUpper(backendName))
			return exitUserError
		}
		fmt.Fprintf(os.Stderr, "qcmd: API error: %v\n", err)
		return exitSystemError
	}

	// Sanitize command.
	command := sanitize.Sanitize(resp.Command)

	// Check for error sentinel.
	if isError, errMsg := sanitize.CheckErrorSentinel(command); isError {
		fmt.Fprintf(os.Stderr, "qcmd: LLM could not generate command: %s\n", errMsg)
		return exitUserError
	}

	if f.verbose {
		fmt.Fprintf(os.Stderr, "qcmd: tokens used: %d\n", resp.TokensUsed)
	}

	// Run safety check (unless disabled).
	var checkResult safety.CheckResult
	isDangerous := false
	if !f.noSafety {
		checker := safety.NewChecker()
		checkResult = checker.Check(command)

		if checkResult.Level == safety.Danger && cfg.Safety.BlockDangerous {
			isDangerous = true
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "WARNING: Dangerous command detected!")
			fmt.Fprintf(os.Stderr, "  Category: %s\n", checkResult.Category)
			fmt.Fprintf(os.Stderr, "  Reason: %s\n", checkResult.Description)
			fmt.Fprintln(os.Stderr, "")
		} else if checkResult.Level == safety.Caution && cfg.Safety.ShowWarnings {
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "Caution: Review this command before executing.")
			fmt.Fprintf(os.Stderr, "  Category: %s\n", checkResult.Category)
			fmt.Fprintf(os.Stderr, "  Reason: %s\n", checkResult.Description)
			fmt.Fprintln(os.Stderr, "")
		}
	}

	// Output the command.
	if err := output.Output(command, outputMode, isDangerous); err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: output error: %v\n", err)
		return exitSystemError
	}

	// Return appropriate exit code.
	if isDangerous {
		return exitDangerBlocked
	}
	return exitSuccess
}

// parseFlags parses command-line flags and returns a flags struct.
func parseFlags(args []string) (*flags, error) {
	f := &flags{}
	fs := flag.NewFlagSet("qcmd", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	fs.StringVar(&f.queryFile, "query-file", "", "Read query from file")
	fs.StringVar(&f.query, "query", "", "Direct query string")
	fs.StringVar(&f.backendStr, "backend", "", "Override backend (anthropic|openai|openrouter)")
	fs.StringVar(&f.model, "model", "", "Override model")
	fs.StringVar(&f.outputMode, "output", "", "Output mode: zle|clipboard|print|auto")
	fs.BoolVar(&f.noSafety, "no-safety", false, "Disable safety checks")
	fs.StringVar(&f.configPath, "config", "", "Config file path")
	fs.BoolVar(&f.verbose, "verbose", false, "Verbose output to stderr")
	fs.BoolVar(&f.showVer, "version", false, "Print version and exit")

	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "qcmd - Natural language to shell command")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  qcmd [flags]")
		fmt.Fprintln(os.Stderr, "  qcmd [command]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		fs.PrintDefaults()
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Input Precedence (highest to lowest):")
		fmt.Fprintln(os.Stderr, "  1. --query-file (if provided)")
		fmt.Fprintln(os.Stderr, "  2. --query (if provided)")
		fmt.Fprintln(os.Stderr, "  3. Interactive editor")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Commands:")
		fmt.Fprintln(os.Stderr, "  config           Show current configuration")
		fmt.Fprintln(os.Stderr, "  config init      Create default config file")
		fmt.Fprintln(os.Stderr, "  backends         List available backends")
	}

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return f, nil
}

// getQuery gets the query string from the appropriate source.
// Precedence: --query-file > --query > editor
func getQuery(f *flags, cfg *config.Config) (string, error) {
	// Priority 1: --query-file
	if f.queryFile != "" {
		content, err := os.ReadFile(f.queryFile)
		if err != nil {
			return "", fmt.Errorf("reading query file: %w", err)
		}
		return editor.ProcessInput(string(content)), nil
	}

	// Priority 2: --query
	if f.query != "" {
		return f.query, nil
	}

	// Priority 3: Interactive editor
	ed := editor.NewEditor(cfg.Editor.Editor)
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout())
	defer cancel()

	query, err := ed.GetInput(ctx)
	if err != nil {
		return "", fmt.Errorf("getting input from editor: %w", err)
	}

	return query, nil
}

// validateInput validates the query string.
func validateInput(query string) error {
	// Check for empty/whitespace-only input.
	if strings.TrimSpace(query) == "" {
		return errors.New("empty query")
	}

	// Check for null bytes (security).
	if strings.ContainsRune(query, 0) {
		return errors.New("invalid input: contains null bytes")
	}

	// Check reasonable length (prevent abuse).
	if len(query) > maxQueryLength {
		return fmt.Errorf("query too long (max %d bytes)", maxQueryLength)
	}

	return nil
}

// createBackend creates an LLM backend based on the configured backend name.
func createBackend(name string, cfg *config.Config) (backend.Backend, error) {
	switch name {
	case "anthropic":
		return backend.NewAnthropicBackend(
			backend.WithAnthropicAPIKey(cfg.Anthropic.APIKey),
			backend.WithAnthropicModel(cfg.Anthropic.Model),
			backend.WithAnthropicMaxTokens(cfg.Advanced.MaxTokens),
		), nil

	case "openai":
		return backend.NewOpenAIBackend(
			backend.WithOpenAIAPIKey(cfg.OpenAI.APIKey),
			backend.WithOpenAIModel(cfg.OpenAI.Model),
			backend.WithOpenAIMaxTokens(cfg.Advanced.MaxTokens),
		), nil

	case "openrouter":
		return backend.NewOpenRouterBackend(
			backend.WithOpenRouterAPIKey(cfg.OpenRouter.APIKey),
			backend.WithOpenRouterModel(cfg.OpenRouter.Model),
			backend.WithOpenRouterMaxTokens(cfg.Advanced.MaxTokens),
		), nil

	default:
		return nil, fmt.Errorf("unknown backend: %s (valid: anthropic, openai, openrouter)", name)
	}
}

// handleConfigCommand handles the 'config' and 'config init' subcommands.
func handleConfigCommand(args []string) int {
	// Check for 'config init' subcommand.
	if len(args) > 0 && args[0] == "init" {
		return handleConfigInit()
	}

	// Show current configuration.
	cfg, err := config.Load(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: failed to load config: %v\n", err)
		return exitSystemError
	}

	fmt.Fprintln(os.Stderr, "Current configuration:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "  Backend:         %s\n", cfg.Backend)
	fmt.Fprintf(os.Stderr, "  Include Context: %t\n", cfg.IncludeContext)
	fmt.Fprintf(os.Stderr, "  Output Mode:     %s\n", cfg.OutputMode)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [anthropic]")
	fmt.Fprintf(os.Stderr, "    Model:         %s\n", cfg.Anthropic.Model)
	fmt.Fprintf(os.Stderr, "    API Key:       %s\n", maskAPIKey(cfg.Anthropic.APIKey))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [openai]")
	fmt.Fprintf(os.Stderr, "    Model:         %s\n", cfg.OpenAI.Model)
	fmt.Fprintf(os.Stderr, "    API Key:       %s\n", maskAPIKey(cfg.OpenAI.APIKey))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [openrouter]")
	fmt.Fprintf(os.Stderr, "    Model:         %s\n", cfg.OpenRouter.Model)
	fmt.Fprintf(os.Stderr, "    API Key:       %s\n", maskAPIKey(cfg.OpenRouter.APIKey))
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [safety]")
	fmt.Fprintf(os.Stderr, "    Block Danger:  %t\n", cfg.Safety.BlockDangerous)
	fmt.Fprintf(os.Stderr, "    Show Warnings: %t\n", cfg.Safety.ShowWarnings)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  [advanced]")
	fmt.Fprintf(os.Stderr, "    Timeout:       %ds\n", cfg.Advanced.TimeoutSeconds)
	fmt.Fprintf(os.Stderr, "    Max Tokens:    %d\n", cfg.Advanced.MaxTokens)

	return exitSuccess
}

// handleConfigInit handles the 'config init' subcommand.
func handleConfigInit() int {
	path, err := config.InitConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: %v\n", err)
		return exitUserError
	}
	fmt.Fprintf(os.Stderr, "Created config file: %s\n", path)
	return exitSuccess
}

// handleBackendsCommand handles the 'backends' subcommand.
func handleBackendsCommand() int {
	cfg, err := config.Load(nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "qcmd: failed to load config: %v\n", err)
		return exitSystemError
	}

	fmt.Fprintln(os.Stderr, "Available backends:")
	fmt.Fprintln(os.Stderr, "")

	// Anthropic
	anthropicStatus := "not configured"
	if cfg.Anthropic.APIKey != "" {
		anthropicStatus = "configured"
	}
	activeMarker := ""
	if cfg.Backend == "anthropic" {
		activeMarker = " (active)"
	}
	fmt.Fprintf(os.Stderr, "  anthropic%s\n", activeMarker)
	fmt.Fprintf(os.Stderr, "    Status: %s\n", anthropicStatus)
	fmt.Fprintf(os.Stderr, "    Model:  %s\n", cfg.Anthropic.Model)
	fmt.Fprintln(os.Stderr, "")

	// OpenAI
	openaiStatus := "not configured"
	if cfg.OpenAI.APIKey != "" {
		openaiStatus = "configured"
	}
	activeMarker = ""
	if cfg.Backend == "openai" {
		activeMarker = " (active)"
	}
	fmt.Fprintf(os.Stderr, "  openai%s\n", activeMarker)
	fmt.Fprintf(os.Stderr, "    Status: %s\n", openaiStatus)
	fmt.Fprintf(os.Stderr, "    Model:  %s\n", cfg.OpenAI.Model)
	fmt.Fprintln(os.Stderr, "")

	// OpenRouter
	openrouterStatus := "not configured"
	if cfg.OpenRouter.APIKey != "" {
		openrouterStatus = "configured"
	}
	activeMarker = ""
	if cfg.Backend == "openrouter" {
		activeMarker = " (active)"
	}
	fmt.Fprintf(os.Stderr, "  openrouter%s\n", activeMarker)
	fmt.Fprintf(os.Stderr, "    Status: %s\n", openrouterStatus)
	fmt.Fprintf(os.Stderr, "    Model:  %s\n", cfg.OpenRouter.Model)

	return exitSuccess
}

// maskAPIKey returns a masked version of an API key for display.
// Never logs or prints the full key.
func maskAPIKey(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
