// Package config handles configuration loading from TOML files and environment variables.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// DefaultConfigTOML is the default configuration template for `config init`.
const DefaultConfigTOML = `# qcmd configuration file
# See: https://github.com/user/qcmd

# Default backend to use: anthropic | openai | openrouter
backend = "anthropic"

# Include shell context (pwd, shell, OS) in prompts
include_context = true

# Output mode preference when run directly (not via shell wrapper)
# "auto" = try clipboard, then print
# "clipboard" = always clipboard
# "print" = always print
output_mode = "auto"

[anthropic]
# API key (or use ANTHROPIC_API_KEY env var)
api_key = ""
# Model to use (any valid Anthropic model)
model = "claude-haiku-4-5-20251001"

[openai]
# API key (or use OPENAI_API_KEY env var)
api_key = ""
# Model to use (any valid OpenAI model)
model = "gpt-5o"

[openrouter]
# API key (or use OPENROUTER_API_KEY env var)
api_key = ""
# Model to use (any model available on OpenRouter)
model = "anthropic/claude-haiku-4-5-20251001"

[safety]
# Block dangerous commands from being injected (still prints them)
block_dangerous = true
# Show warnings for cautionary commands
show_warnings = true

[editor]
# Override $EDITOR/$VISUAL (uncomment to use)
# editor = "nvim"

[advanced]
# API call timeout in seconds
timeout_seconds = 30
# Maximum tokens for LLM response
max_tokens = 512
`

// Config represents the full configuration for qcmd.
type Config struct {
	Backend        string          `toml:"backend"`
	IncludeContext bool            `toml:"include_context"`
	OutputMode     string          `toml:"output_mode"`
	Anthropic      AnthropicConfig `toml:"anthropic"`
	OpenAI         OpenAIConfig    `toml:"openai"`
	OpenRouter     OpenRouterConfig `toml:"openrouter"`
	Safety         SafetyConfig    `toml:"safety"`
	Editor         EditorConfig    `toml:"editor"`
	Advanced       AdvancedConfig  `toml:"advanced"`
}

// AnthropicConfig holds Anthropic-specific configuration.
type AnthropicConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// OpenAIConfig holds OpenAI-specific configuration.
type OpenAIConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// OpenRouterConfig holds OpenRouter-specific configuration.
type OpenRouterConfig struct {
	APIKey string `toml:"api_key"`
	Model  string `toml:"model"`
}

// SafetyConfig holds safety check configuration.
type SafetyConfig struct {
	BlockDangerous bool `toml:"block_dangerous"`
	ShowWarnings   bool `toml:"show_warnings"`
}

// EditorConfig holds editor configuration.
type EditorConfig struct {
	Editor string `toml:"editor"`
}

// AdvancedConfig holds advanced configuration options.
type AdvancedConfig struct {
	TimeoutSeconds int `toml:"timeout_seconds"`
	MaxTokens      int `toml:"max_tokens"`
}

// Timeout returns the configured timeout as a time.Duration.
func (c *Config) Timeout() time.Duration {
	return time.Duration(c.Advanced.TimeoutSeconds) * time.Second
}

// Default returns a Config with sensible default values.
func Default() *Config {
	return &Config{
		Backend:        "anthropic",
		IncludeContext: true,
		OutputMode:     "auto",
		Anthropic: AnthropicConfig{
			Model: "claude-haiku-4-5-20251001",
		},
		OpenAI: OpenAIConfig{
			Model: "gpt-5o",
		},
		OpenRouter: OpenRouterConfig{
			Model: "anthropic/claude-haiku-4-5-20251001",
		},
		Safety: SafetyConfig{
			BlockDangerous: true,
			ShowWarnings:   true,
		},
		Advanced: AdvancedConfig{
			TimeoutSeconds: 30,
			MaxTokens:      512,
		},
	}
}

// LoadOptions configures how configuration is loaded.
type LoadOptions struct {
	// ConfigPath is an explicit path to a config file (highest priority).
	ConfigPath string
}

// Load loads configuration from the appropriate source with the following priority:
// 1. --config flag (via LoadOptions.ConfigPath)
// 2. $QCMD_CONFIG env var
// 3. $XDG_CONFIG_HOME/qcmd/config.toml
// 4. ~/.config/qcmd/config.toml
//
// Environment variables override file config for API keys and backend selection.
func Load(opts *LoadOptions) (*Config, error) {
	cfg := Default()

	// Determine config file path
	configPath := findConfigPath(opts)

	// Load from file if it exists
	if configPath != "" {
		if err := loadFromFile(cfg, configPath); err != nil {
			return nil, fmt.Errorf("loading config file: %w", err)
		}
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	return cfg, nil
}

// findConfigPath determines the config file path based on priority.
func findConfigPath(opts *LoadOptions) string {
	// Priority 1: Explicit path from --config flag
	if opts != nil && opts.ConfigPath != "" {
		return opts.ConfigPath
	}

	// Priority 2: QCMD_CONFIG env var
	if envPath := os.Getenv("QCMD_CONFIG"); envPath != "" {
		return envPath
	}

	// Priority 3: $XDG_CONFIG_HOME/qcmd/config.toml
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		xdgPath := filepath.Join(xdgConfigHome, "qcmd", "config.toml")
		if fileExists(xdgPath) {
			return xdgPath
		}
	}

	// Priority 4: ~/.config/qcmd/config.toml
	if homeDir, err := os.UserHomeDir(); err == nil {
		homePath := filepath.Join(homeDir, ".config", "qcmd", "config.toml")
		if fileExists(homePath) {
			return homePath
		}
	}

	return ""
}

// loadFromFile loads configuration from a TOML file.
func loadFromFile(cfg *Config, path string) error {
	// Check file permissions and warn if not 0600
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat config file: %w", err)
	}

	// Check permissions (Unix-only, ignore on Windows)
	mode := info.Mode().Perm()
	if mode&0077 != 0 {
		// File is readable by group or others - warn to stderr
		fmt.Fprintf(os.Stderr, "warning: config file %s has insecure permissions %o, should be 0600\n", path, mode)
	}

	// Parse TOML
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return fmt.Errorf("parse TOML: %w", err)
	}

	return nil
}

// applyEnvOverrides applies environment variable overrides to the config.
// Environment variables take precedence over file config.
func applyEnvOverrides(cfg *Config) {
	// API keys from environment
	if key := os.Getenv("ANTHROPIC_API_KEY"); key != "" {
		cfg.Anthropic.APIKey = key
	}
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		cfg.OpenAI.APIKey = key
	}
	if key := os.Getenv("OPENROUTER_API_KEY"); key != "" {
		cfg.OpenRouter.APIKey = key
	}

	// Backend override from environment
	if backend := os.Getenv("QCMD_BACKEND"); backend != "" {
		cfg.Backend = backend
	}
}

// fileExists returns true if the file at path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// GetConfigDir returns the directory where config should be stored.
// Uses $XDG_CONFIG_HOME/qcmd if set, otherwise ~/.config/qcmd.
func GetConfigDir() (string, error) {
	if xdgConfigHome := os.Getenv("XDG_CONFIG_HOME"); xdgConfigHome != "" {
		return filepath.Join(xdgConfigHome, "qcmd"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}

	return filepath.Join(homeDir, ".config", "qcmd"), nil
}

// InitConfig creates a default configuration file at the standard location.
// Returns an error if the file already exists.
func InitConfig() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	// Create directory with secure permissions if it doesn't exist
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return "", fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.toml")

	// Don't overwrite existing config
	if fileExists(configPath) {
		return "", fmt.Errorf("config file already exists: %s", configPath)
	}

	// Write default config with 0600 permissions
	if err := os.WriteFile(configPath, []byte(DefaultConfigTOML), 0600); err != nil {
		return "", fmt.Errorf("writing config file: %w", err)
	}

	return configPath, nil
}

// GetAPIKey returns the API key for the specified backend.
// Returns empty string if no key is configured.
func (c *Config) GetAPIKey(backend string) string {
	switch backend {
	case "anthropic":
		return c.Anthropic.APIKey
	case "openai":
		return c.OpenAI.APIKey
	case "openrouter":
		return c.OpenRouter.APIKey
	default:
		return ""
	}
}

// GetModel returns the model for the specified backend.
func (c *Config) GetModel(backend string) string {
	switch backend {
	case "anthropic":
		return c.Anthropic.Model
	case "openai":
		return c.OpenAI.Model
	case "openrouter":
		return c.OpenRouter.Model
	default:
		return ""
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	// Validate backend
	switch c.Backend {
	case "anthropic", "openai", "openrouter":
		// valid
	default:
		return fmt.Errorf("invalid backend: %s (must be anthropic, openai, or openrouter)", c.Backend)
	}

	// Validate output mode
	switch c.OutputMode {
	case "auto", "clipboard", "print", "zle":
		// valid
	default:
		return fmt.Errorf("invalid output_mode: %s (must be auto, clipboard, print, or zle)", c.OutputMode)
	}

	// Validate timeout
	if c.Advanced.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeout_seconds must be positive")
	}

	// Validate max_tokens
	if c.Advanced.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}

	return nil
}
