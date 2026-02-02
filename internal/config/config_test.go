package config

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	
	configContent := `
server:
  listen: "127.0.0.1:9999"
  audit: "/tmp/audit.log"

auth:
  tokens:
    - "test-token-1"
    - "test-token-2"
  allowed_ips:
    - "127.0.0.1"
    - "100.100.0.0/16"

tools:
  echo:
    path: /bin/echo
    pass_args: true
  
  restricted:
    path: /bin/cat
    args_pattern: "^[a-z]+\\.txt$"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	
	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	
	// Verify server config
	if cfg.Server.Listen != "127.0.0.1:9999" {
		t.Errorf("wrong listen: %s", cfg.Server.Listen)
	}
	
	// Verify auth
	if len(cfg.Auth.Tokens) != 2 {
		t.Errorf("wrong token count: %d", len(cfg.Auth.Tokens))
	}
	
	// Verify tools
	if len(cfg.Tools) != 2 {
		t.Errorf("wrong tool count: %d", len(cfg.Tools))
	}
	
	echo, ok := cfg.Tools["echo"]
	if !ok {
		t.Fatal("echo tool not found")
	}
	if echo.Path != "/bin/echo" {
		t.Errorf("wrong path: %s", echo.Path)
	}
	if !echo.PassArgs {
		t.Error("echo should have pass_args=true")
	}
}

func TestToolValidateArgs(t *testing.T) {
	tests := []struct {
		name        string
		tool        Tool
		args        []string
		shouldError bool
	}{
		{
			name:        "pass_args allows anything",
			tool:        Tool{PassArgs: true},
			args:        []string{"--flag", "value", "anything"},
			shouldError: false,
		},
		{
			name:        "pattern matches valid args",
			tool:        Tool{ArgsPattern: "^[a-z]+$", argsRegex: mustCompile("^[a-z]+$")},
			args:        []string{"hello", "world"},
			shouldError: false,
		},
		{
			name:        "pattern rejects invalid args",
			tool:        Tool{ArgsPattern: "^[a-z]+$", argsRegex: mustCompile("^[a-z]+$")},
			args:        []string{"hello", "WORLD"},
			shouldError: true,
		},
		{
			name:        "empty args always ok",
			tool:        Tool{ArgsPattern: "^[a-z]+$", argsRegex: mustCompile("^[a-z]+$")},
			args:        []string{},
			shouldError: false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.tool.ValidateArgs(tt.args)
			if tt.shouldError && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestLoadCredentials(t *testing.T) {
	dir := t.TempDir()
	credsPath := filepath.Join(dir, "creds.yaml")
	
	credsContent := `
api-key: "secret-key-123"
password: "hunter2"
`
	if err := os.WriteFile(credsPath, []byte(credsContent), 0600); err != nil {
		t.Fatalf("write creds: %v", err)
	}
	
	creds, err := LoadCredentials(credsPath)
	if err != nil {
		t.Fatalf("load creds: %v", err)
	}
	
	if creds["api-key"] != "secret-key-123" {
		t.Errorf("wrong api-key: %s", creds["api-key"])
	}
	if creds["password"] != "hunter2" {
		t.Errorf("wrong password: %s", creds["password"])
	}
}

func mustCompile(pattern string) *regexp.Regexp {
	return regexp.MustCompile(pattern)
}
