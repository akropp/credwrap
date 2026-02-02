package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ToolAddOptions holds options for toolsAdd
type ToolAddOptions struct {
	Symlink bool // Create symlink instead of copying
	NoCopy  bool // Don't copy, just add to config with original path
}

// toolsAdd copies/symlinks a tool to /usr/local/bin and adds it to the config
func toolsAdd(configPath, toolName, sourcePath string, credentialEnvs []string, opts ToolAddOptions) error {
	// Validate source exists
	sourcePath, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}
	
	info, err := os.Stat(sourcePath)
	if err != nil {
		return fmt.Errorf("source not found: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("source is a directory, expected executable")
	}

	var finalPath string

	if opts.NoCopy {
		// Just use the original path
		finalPath = sourcePath
		fmt.Printf("Using original path: %s\n", finalPath)
		fmt.Println("Note: credwrap user must have execute permission on this path")
	} else {
		// Determine destination
		destDir := "/usr/local/bin"
		destPath := filepath.Join(destDir, filepath.Base(sourcePath))

		// Check if we can write to dest (need sudo)
		if err := checkWritable(destDir); err != nil {
			return fmt.Errorf("cannot write to %s (try running with sudo): %w", destDir, err)
		}

		if opts.Symlink {
			// Create symlink
			fmt.Printf("Symlinking %s -> %s\n", destPath, sourcePath)
			
			// Remove existing file/symlink if present
			os.Remove(destPath)
			
			if err := os.Symlink(sourcePath, destPath); err != nil {
				return fmt.Errorf("creating symlink: %w", err)
			}
			fmt.Println("Note: credwrap user must have execute permission on the source path")
		} else {
			// Copy the file
			fmt.Printf("Copying %s -> %s\n", sourcePath, destPath)
			if err := copyFile(sourcePath, destPath); err != nil {
				return fmt.Errorf("copying file: %w", err)
			}

			// Make executable
			if err := os.Chmod(destPath, 0755); err != nil {
				return fmt.Errorf("chmod: %w", err)
			}
		}
		
		finalPath = destPath
	}

	// Update config
	fmt.Printf("Updating config: %s\n", configPath)
	if err := addToolToConfig(configPath, toolName, finalPath, credentialEnvs); err != nil {
		return fmt.Errorf("updating config: %w", err)
	}

	fmt.Printf("✓ Tool '%s' added successfully\n", toolName)
	fmt.Println("")
	fmt.Println("Next steps:")
	if len(credentialEnvs) > 0 {
		fmt.Println("  1. Add the required secrets:")
		for _, env := range credentialEnvs {
			secretName := envToSecretName(env)
			fmt.Printf("     credwrap-server secrets add <credentials-file> %s\n", secretName)
		}
		fmt.Println("  2. Restart the server:")
		fmt.Println("     sudo systemctl restart credwrap")
	} else {
		fmt.Println("  1. Restart the server:")
		fmt.Println("     sudo systemctl restart credwrap")
	}

	return nil
}

// toolsList lists tools in the config
func toolsList(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	var cfg struct {
		Tools map[string]struct {
			Path        string `yaml:"path"`
			Credentials []struct {
				Env    string `yaml:"env"`
				Secret string `yaml:"secret"`
			} `yaml:"credentials"`
		} `yaml:"tools"`
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	fmt.Printf("Tools in %s:\n\n", configPath)
	for name, tool := range cfg.Tools {
		fmt.Printf("  %s\n", name)
		fmt.Printf("    path: %s\n", tool.Path)
		if len(tool.Credentials) > 0 {
			fmt.Printf("    credentials:\n")
			for _, cred := range tool.Credentials {
				fmt.Printf("      - %s (secret: %s)\n", cred.Env, cred.Secret)
			}
		}
		fmt.Println()
	}

	return nil
}

// toolsRemove removes a tool from the config (doesn't delete the binary)
func toolsRemove(configPath, toolName string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	tools, ok := cfg["tools"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no tools section in config")
	}

	if _, exists := tools[toolName]; !exists {
		return fmt.Errorf("tool '%s' not found in config", toolName)
	}

	delete(tools, toolName)

	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	fmt.Printf("✓ Tool '%s' removed from config\n", toolName)
	fmt.Println("  Note: Binary was not deleted. Restart server to apply changes.")
	return nil
}

func addToolToConfig(configPath, toolName, toolPath string, credentialEnvs []string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Get or create tools section
	tools, ok := cfg["tools"].(map[string]interface{})
	if !ok {
		tools = make(map[string]interface{})
		cfg["tools"] = tools
	}

	// Build tool entry
	toolEntry := map[string]interface{}{
		"path":      toolPath,
		"pass_args": true,
	}

	// Add credentials if specified
	if len(credentialEnvs) > 0 {
		var creds []map[string]string
		for _, env := range credentialEnvs {
			creds = append(creds, map[string]string{
				"env":    env,
				"secret": envToSecretName(env),
			})
		}
		toolEntry["credentials"] = creds
	} else {
		toolEntry["credentials"] = []interface{}{}
	}

	tools[toolName] = toolEntry

	// Write back
	newData, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("serializing config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dest.Close()

	_, err = io.Copy(dest, source)
	return err
}

func checkWritable(dir string) error {
	testFile := filepath.Join(dir, ".credwrap-write-test")
	f, err := os.Create(testFile)
	if err != nil {
		return err
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// envToSecretName converts ENV_VAR_NAME to env-var-name
func envToSecretName(env string) string {
	return strings.ToLower(strings.ReplaceAll(env, "_", "-"))
}
