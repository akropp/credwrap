// Package config handles credwrap configuration parsing.
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"

	"filippo.io/age"
	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration.
type Config struct {
	Server      ServerConfig        `yaml:"server"`
	Auth        AuthConfig          `yaml:"auth"`
	Tools       map[string]Tool     `yaml:"tools"`
	Credentials map[string]string   `yaml:"-"` // Loaded separately from encrypted file
}

// ServerConfig defines server binding options.
type ServerConfig struct {
	Listen string `yaml:"listen"` // e.g., "127.0.0.1:9876" or "100.100.132.22:9876"
	Audit  string `yaml:"audit"`  // Path to audit log file (optional)
}

// AuthConfig defines authentication options.
type AuthConfig struct {
	Tokens         []string `yaml:"tokens"`           // Allowed tokens
	TailscaleNodes []string `yaml:"tailscale_nodes"`  // Allowed Tailscale node IDs (optional)
	AllowedIPs     []string `yaml:"allowed_ips"`      // Allowed IP addresses or CIDR ranges
	RequireToken   bool     `yaml:"require_token"`    // If false, IP/Tailscale auth alone is sufficient
}

// Tool defines an allowed tool and its credential mappings.
type Tool struct {
	Path        string       `yaml:"path"`                   // Full path to executable
	Credentials []Credential `yaml:"credentials,omitempty"`  // Credentials to inject
	PassArgs    bool         `yaml:"pass_args"`              // Allow arbitrary args
	ArgsPattern string       `yaml:"args_pattern,omitempty"` // Regex to validate args

	argsRegex *regexp.Regexp // Compiled regex
}

// Credential defines how to inject a credential.
type Credential struct {
	Env    string `yaml:"env,omitempty"`    // Set as environment variable
	Header string `yaml:"header,omitempty"` // For HTTP tools, add as header (future)
	Flag   string `yaml:"flag,omitempty"`   // Add as command-line flag (future)
	Secret string `yaml:"secret"`           // Key in credentials store
}

// LoadConfig loads the configuration from a YAML file.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Compile args patterns
	for name, tool := range cfg.Tools {
		if tool.ArgsPattern != "" {
			regex, err := regexp.Compile(tool.ArgsPattern)
			if err != nil {
				return nil, fmt.Errorf("invalid args_pattern for tool %s: %w", name, err)
			}
			tool.argsRegex = regex
			cfg.Tools[name] = tool
		}
	}

	// Set defaults
	if cfg.Server.Listen == "" {
		cfg.Server.Listen = "127.0.0.1:9876"
	}

	return &cfg, nil
}

// ValidateArgs checks if the given args are allowed for this tool.
func (t *Tool) ValidateArgs(args []string) error {
	if t.PassArgs {
		return nil
	}
	if t.argsRegex != nil {
		for _, arg := range args {
			if !t.argsRegex.MatchString(arg) {
				return fmt.Errorf("argument %q does not match allowed pattern", arg)
			}
		}
	}
	return nil
}

// LoadCredentials loads and decrypts the credentials file.
// For now, this expects a plaintext YAML file. Age encryption will be added.
func LoadCredentials(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading credentials: %w", err)
	}

	var creds map[string]string
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return creds, nil
}

// LoadCredentialsEncrypted loads credentials from an age-encrypted file.
func LoadCredentialsEncrypted(path string, password string) (map[string]string, error) {
	// Read encrypted file
	encData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading encrypted credentials: %w", err)
	}

	// Create identity from password
	identity, err := age.NewScryptIdentity(password)
	if err != nil {
		return nil, fmt.Errorf("creating identity: %w", err)
	}

	// Decrypt
	reader, err := age.Decrypt(bytes.NewReader(encData), identity)
	if err != nil {
		return nil, fmt.Errorf("decrypting credentials (wrong password?): %w", err)
	}

	// Read decrypted data
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("reading decrypted data: %w", err)
	}

	// Parse YAML
	var creds map[string]string
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}

	return creds, nil
}
