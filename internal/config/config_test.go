package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"backend", cfg.Backend, "anthropic"},
		{"include_context", cfg.IncludeContext, true},
		{"output_mode", cfg.OutputMode, "auto"},
		{"anthropic.model", cfg.Anthropic.Model, "claude-4-haiku"},
		{"openai.model", cfg.OpenAI.Model, "gpt-5o"},
		{"openrouter.model", cfg.OpenRouter.Model, "anthropic/claude-4-haiku"},
		{"safety.block_dangerous", cfg.Safety.BlockDangerous, true},
		{"safety.show_warnings", cfg.Safety.ShowWarnings, true},
		{"advanced.timeout_seconds", cfg.Advanced.TimeoutSeconds, 30},
		{"advanced.max_tokens", cfg.Advanced.MaxTokens, 512},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Default().%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestTimeout(t *testing.T) {
	cfg := Default()
	timeout := cfg.Timeout()

	if timeout.Seconds() != 30 {
		t.Errorf("Timeout() = %v, want 30s", timeout)
	}
}

func TestLoadFromTOML(t *testing.T) {
	// Create a temporary TOML file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `
backend = "openai"
include_context = false
output_mode = "clipboard"

[anthropic]
api_key = "test-anthropic-key"
model = "claude-4-sonnet"

[openai]
api_key = "test-openai-key"
model = "gpt-4o"

[openrouter]
api_key = "test-openrouter-key"
model = "meta-llama/llama-3-70b"

[safety]
block_dangerous = false
show_warnings = false

[editor]
editor = "code --wait"

[advanced]
timeout_seconds = 60
max_tokens = 1024
`

	// Write with secure permissions
	if err := os.WriteFile(configPath, []byte(tomlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(&LoadOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"backend", cfg.Backend, "openai"},
		{"include_context", cfg.IncludeContext, false},
		{"output_mode", cfg.OutputMode, "clipboard"},
		{"anthropic.api_key", cfg.Anthropic.APIKey, "test-anthropic-key"},
		{"anthropic.model", cfg.Anthropic.Model, "claude-4-sonnet"},
		{"openai.api_key", cfg.OpenAI.APIKey, "test-openai-key"},
		{"openai.model", cfg.OpenAI.Model, "gpt-4o"},
		{"openrouter.api_key", cfg.OpenRouter.APIKey, "test-openrouter-key"},
		{"openrouter.model", cfg.OpenRouter.Model, "meta-llama/llama-3-70b"},
		{"safety.block_dangerous", cfg.Safety.BlockDangerous, false},
		{"safety.show_warnings", cfg.Safety.ShowWarnings, false},
		{"editor.editor", cfg.Editor.Editor, "code --wait"},
		{"advanced.timeout_seconds", cfg.Advanced.TimeoutSeconds, 60},
		{"advanced.max_tokens", cfg.Advanced.MaxTokens, 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("loaded config.%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	// Create a temporary TOML file with some values
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	tomlContent := `
backend = "anthropic"

[anthropic]
api_key = "file-key"

[openai]
api_key = "file-openai-key"

[openrouter]
api_key = "file-openrouter-key"
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Set environment variables
	t.Setenv("ANTHROPIC_API_KEY", "env-anthropic-key")
	t.Setenv("OPENAI_API_KEY", "env-openai-key")
	t.Setenv("OPENROUTER_API_KEY", "env-openrouter-key")
	t.Setenv("QCMD_BACKEND", "openrouter")

	cfg, err := Load(&LoadOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	tests := []struct {
		name     string
		got      string
		expected string
	}{
		{"anthropic.api_key", cfg.Anthropic.APIKey, "env-anthropic-key"},
		{"openai.api_key", cfg.OpenAI.APIKey, "env-openai-key"},
		{"openrouter.api_key", cfg.OpenRouter.APIKey, "env-openrouter-key"},
		{"backend", cfg.Backend, "openrouter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("env override %s = %q, want %q", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestConfigPathPriority(t *testing.T) {
	// Test priority 1: explicit ConfigPath
	t.Run("explicit path takes priority", func(t *testing.T) {
		tmpDir := t.TempDir()
		explicitPath := filepath.Join(tmpDir, "explicit.toml")
		envPath := filepath.Join(tmpDir, "env.toml")

		// Write different backends to each file to distinguish them
		if err := os.WriteFile(explicitPath, []byte(`backend = "openai"`), 0600); err != nil {
			t.Fatalf("failed to write explicit config: %v", err)
		}
		if err := os.WriteFile(envPath, []byte(`backend = "openrouter"`), 0600); err != nil {
			t.Fatalf("failed to write env config: %v", err)
		}

		t.Setenv("QCMD_CONFIG", envPath)

		cfg, err := Load(&LoadOptions{ConfigPath: explicitPath})
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Backend != "openai" {
			t.Errorf("explicit path should take priority, got backend = %q, want %q", cfg.Backend, "openai")
		}
	})

	// Test priority 2: QCMD_CONFIG env var
	t.Run("QCMD_CONFIG takes priority over XDG", func(t *testing.T) {
		tmpDir := t.TempDir()
		envPath := filepath.Join(tmpDir, "env.toml")
		xdgDir := filepath.Join(tmpDir, "xdg", "qcmd")
		xdgPath := filepath.Join(xdgDir, "config.toml")

		if err := os.WriteFile(envPath, []byte(`backend = "openai"`), 0600); err != nil {
			t.Fatalf("failed to write env config: %v", err)
		}
		if err := os.MkdirAll(xdgDir, 0700); err != nil {
			t.Fatalf("failed to create XDG dir: %v", err)
		}
		if err := os.WriteFile(xdgPath, []byte(`backend = "openrouter"`), 0600); err != nil {
			t.Fatalf("failed to write XDG config: %v", err)
		}

		t.Setenv("QCMD_CONFIG", envPath)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "xdg"))

		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Backend != "openai" {
			t.Errorf("QCMD_CONFIG should take priority, got backend = %q, want %q", cfg.Backend, "openai")
		}
	})

	// Test priority 3: XDG_CONFIG_HOME
	t.Run("XDG_CONFIG_HOME used when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		xdgDir := filepath.Join(tmpDir, "xdg", "qcmd")
		xdgPath := filepath.Join(xdgDir, "config.toml")

		if err := os.MkdirAll(xdgDir, 0700); err != nil {
			t.Fatalf("failed to create XDG dir: %v", err)
		}
		if err := os.WriteFile(xdgPath, []byte(`backend = "openrouter"`), 0600); err != nil {
			t.Fatalf("failed to write XDG config: %v", err)
		}

		// Clear QCMD_CONFIG to ensure it's not used
		t.Setenv("QCMD_CONFIG", "")
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(tmpDir, "xdg"))

		cfg, err := Load(nil)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Backend != "openrouter" {
			t.Errorf("XDG_CONFIG_HOME should be used, got backend = %q, want %q", cfg.Backend, "openrouter")
		}
	})
}

func TestLoadWithMissingFile(t *testing.T) {
	// When no config file exists, defaults should be used
	t.Setenv("QCMD_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, err := Load(nil)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Should have default values
	if cfg.Backend != "anthropic" {
		t.Errorf("default backend = %q, want %q", cfg.Backend, "anthropic")
	}
}

func TestLoadWithInvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Write invalid TOML
	if err := os.WriteFile(configPath, []byte(`backend = "unclosed`), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_, err := Load(&LoadOptions{ConfigPath: configPath})
	if err == nil {
		t.Error("Load() should return error for invalid TOML")
	}
}

func TestGetAPIKey(t *testing.T) {
	cfg := Default()
	cfg.Anthropic.APIKey = "anthropic-key"
	cfg.OpenAI.APIKey = "openai-key"
	cfg.OpenRouter.APIKey = "openrouter-key"

	tests := []struct {
		backend  string
		expected string
	}{
		{"anthropic", "anthropic-key"},
		{"openai", "openai-key"},
		{"openrouter", "openrouter-key"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			got := cfg.GetAPIKey(tt.backend)
			if got != tt.expected {
				t.Errorf("GetAPIKey(%q) = %q, want %q", tt.backend, got, tt.expected)
			}
		})
	}
}

func TestGetModel(t *testing.T) {
	cfg := Default()
	cfg.Anthropic.Model = "claude-custom"
	cfg.OpenAI.Model = "gpt-custom"
	cfg.OpenRouter.Model = "router-custom"

	tests := []struct {
		backend  string
		expected string
	}{
		{"anthropic", "claude-custom"},
		{"openai", "gpt-custom"},
		{"openrouter", "router-custom"},
		{"invalid", ""},
	}

	for _, tt := range tests {
		t.Run(tt.backend, func(t *testing.T) {
			got := cfg.GetModel(tt.backend)
			if got != tt.expected {
				t.Errorf("GetModel(%q) = %q, want %q", tt.backend, got, tt.expected)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name      string
		modify    func(*Config)
		wantError bool
	}{
		{
			name:      "valid defaults",
			modify:    func(c *Config) {},
			wantError: false,
		},
		{
			name:      "invalid backend",
			modify:    func(c *Config) { c.Backend = "invalid" },
			wantError: true,
		},
		{
			name:      "invalid output_mode",
			modify:    func(c *Config) { c.OutputMode = "invalid" },
			wantError: true,
		},
		{
			name:      "zero timeout",
			modify:    func(c *Config) { c.Advanced.TimeoutSeconds = 0 },
			wantError: true,
		},
		{
			name:      "negative timeout",
			modify:    func(c *Config) { c.Advanced.TimeoutSeconds = -1 },
			wantError: true,
		},
		{
			name:      "zero max_tokens",
			modify:    func(c *Config) { c.Advanced.MaxTokens = 0 },
			wantError: true,
		},
		{
			name:      "valid anthropic backend",
			modify:    func(c *Config) { c.Backend = "anthropic" },
			wantError: false,
		},
		{
			name:      "valid openai backend",
			modify:    func(c *Config) { c.Backend = "openai" },
			wantError: false,
		},
		{
			name:      "valid openrouter backend",
			modify:    func(c *Config) { c.Backend = "openrouter" },
			wantError: false,
		},
		{
			name:      "valid zle output_mode",
			modify:    func(c *Config) { c.OutputMode = "zle" },
			wantError: false,
		},
		{
			name:      "valid clipboard output_mode",
			modify:    func(c *Config) { c.OutputMode = "clipboard" },
			wantError: false,
		},
		{
			name:      "valid print output_mode",
			modify:    func(c *Config) { c.OutputMode = "print" },
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Default()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestInitConfig(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// First init should succeed
	path, err := InitConfig()
	if err != nil {
		t.Fatalf("InitConfig() error = %v", err)
	}

	expectedPath := filepath.Join(tmpDir, "qcmd", "config.toml")
	if path != expectedPath {
		t.Errorf("InitConfig() path = %q, want %q", path, expectedPath)
	}

	// Verify file exists with correct permissions
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("config file permissions = %o, want 0600", perm)
	}

	// Second init should fail (file exists)
	_, err = InitConfig()
	if err == nil {
		t.Error("InitConfig() should fail when file already exists")
	}
}

func TestPartialTOMLConfig(t *testing.T) {
	// Test that partial configs merge with defaults
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.toml")

	// Only specify some values
	tomlContent := `
backend = "openai"

[openai]
api_key = "my-key"
`
	if err := os.WriteFile(configPath, []byte(tomlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(&LoadOptions{ConfigPath: configPath})
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Specified values should be set
	if cfg.Backend != "openai" {
		t.Errorf("backend = %q, want %q", cfg.Backend, "openai")
	}
	if cfg.OpenAI.APIKey != "my-key" {
		t.Errorf("openai.api_key = %q, want %q", cfg.OpenAI.APIKey, "my-key")
	}

	// Unspecified values should use defaults
	if cfg.IncludeContext != true {
		t.Errorf("include_context = %v, want %v", cfg.IncludeContext, true)
	}
	if cfg.OpenAI.Model != "gpt-5o" {
		t.Errorf("openai.model = %q, want %q", cfg.OpenAI.Model, "gpt-5o")
	}
	if cfg.Advanced.TimeoutSeconds != 30 {
		t.Errorf("advanced.timeout_seconds = %d, want %d", cfg.Advanced.TimeoutSeconds, 30)
	}
}
