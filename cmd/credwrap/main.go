// credwrap is the credential wrapper client.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/openclaw/credwrap/internal/client"
	"gopkg.in/yaml.v3"
)

const version = "0.1.0"

func main() {
	// Flags
	serverAddr := flag.String("server", "", "Server address (overrides config)")
	token := flag.String("token", "", "Auth token (overrides config)")
	configPath := flag.String("config", "", "Path to client config file")
	interactive := flag.Bool("i", false, "Interactive mode (forward stdin)")
	ping := flag.Bool("ping", false, "Ping the server and exit")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("credwrap version %s\n", version)
		os.Exit(0)
	}

	// Load config
	cfg := loadConfig(*configPath)

	// Override with flags
	if *serverAddr != "" {
		cfg.Server = *serverAddr
	}
	if *token != "" {
		cfg.Token = *token
	}

	// Validate
	if cfg.Server == "" {
		log.Fatal("Server address required (use -server or config file)")
	}
	if cfg.Token == "" {
		log.Fatal("Auth token required (use -token or config file)")
	}

	// Create client
	c := client.New(cfg.Server, cfg.Token)
	if err := c.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer c.Close()

	// Ping mode
	if *ping {
		version, err := c.Ping()
		if err != nil {
			log.Fatalf("Ping failed: %v", err)
		}
		fmt.Printf("Server version: %s\n", version)
		os.Exit(0)
	}

	// Exec mode
	args := flag.Args()
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: credwrap [flags] <tool> [args...]")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Flags:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	tool := args[0]
	toolArgs := args[1:]

	var exitCode int
	var err error
	if *interactive {
		exitCode, err = c.ExecInteractive(tool, toolArgs)
	} else {
		exitCode, err = c.Exec(tool, toolArgs)
	}

	if err != nil {
		log.Fatalf("Exec failed: %v", err)
	}
	os.Exit(exitCode)
}

func loadConfig(path string) client.ClientConfig {
	var cfg client.ClientConfig

	// Try explicit path
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			log.Fatalf("Failed to read config: %v", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			log.Fatalf("Failed to parse config: %v", err)
		}
		return cfg
	}

	// Try default locations
	home, _ := os.UserHomeDir()
	paths := []string{
		"credwrap.yaml",
		filepath.Join(home, ".credwrap.yaml"),
		filepath.Join(home, ".config", "credwrap", "client.yaml"),
		"/etc/credwrap/client.yaml",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			continue
		}
		return cfg
	}

	// No config found, will need flags
	return cfg
}
