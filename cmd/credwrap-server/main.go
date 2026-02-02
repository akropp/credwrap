// credwrap-server is the credential wrapper daemon.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/openclaw/credwrap/internal/config"
	"github.com/openclaw/credwrap/internal/server"
	"golang.org/x/term"
)

const version = "1.0.0"

func printUsage() {
	fmt.Println(`credwrap-server - Secure credential injection server

Usage:
  credwrap-server [flags]              Start the server

Secrets management:
  credwrap-server secrets init FILE    Create new encrypted credentials file
  credwrap-server secrets add FILE KEY Add a secret (never touches disk in plaintext)
  credwrap-server secrets list FILE    List secret names
  credwrap-server secrets rm FILE KEY  Remove a secret

Tools management:
  credwrap-server tools add CONFIG NAME PATH [--env VAR]...
                                       Copy tool to /usr/local/bin and add to config
  credwrap-server tools list CONFIG    List configured tools
  credwrap-server tools rm CONFIG NAME Remove tool from config

Server flags:`)
	flag.PrintDefaults()
}

func main() {
	// Handle subcommands first
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "secrets":
			handleSecretsCommand()
			return
		case "tools":
			handleToolsCommand()
			return
		case "version", "--version", "-v":
			fmt.Printf("credwrap-server version %s\n", version)
			return
		case "help", "--help", "-h":
			printUsage()
			return
		}
	}

	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	credsPath := flag.String("credentials", "credentials.yaml", "Path to credentials file")
	encrypted := flag.Bool("encrypted", false, "Credentials file is age-encrypted")
	keyfile := flag.String("keyfile", "", "Path to keyfile for decryption (alternative to password prompt)")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load credentials
	var creds map[string]string
	if *encrypted {
		var password string
		if *keyfile != "" {
			// Read password from keyfile
			data, err := os.ReadFile(*keyfile)
			if err != nil {
				log.Fatalf("Failed to read keyfile: %v", err)
			}
			password = strings.TrimSpace(string(data))
		} else {
			// Prompt for password
			fmt.Print("Enter decryption password: ")
			pwBytes, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				log.Fatalf("Failed to read password: %v", err)
			}
			fmt.Println()
			password = string(pwBytes)
		}

		creds, err = config.LoadCredentialsEncrypted(*credsPath, password)
		if err != nil {
			log.Fatalf("Failed to load credentials: %v", err)
		}
	} else {
		creds, err = config.LoadCredentials(*credsPath)
		if err != nil {
			log.Fatalf("Failed to load credentials: %v", err)
		}
	}
	cfg.Credentials = creds

	// Create and start server
	srv := server.New(cfg)

	// Handle shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		log.Println("Shutting down...")
		srv.Stop()
		os.Exit(0)
	}()

	if err := srv.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleSecretsCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: credwrap-server secrets <command> [args] [--keyfile FILE]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  init FILE        Create new encrypted credentials file")
		fmt.Println("  add FILE KEY     Add/update a secret")
		fmt.Println("  list FILE        List secret names (not values)")
		fmt.Println("  rm FILE KEY      Remove a secret")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  --keyfile FILE   Use password from keyfile instead of prompting")
		fmt.Println("")
		fmt.Println("Auto-detection: if no --keyfile is given, looks for:")
		fmt.Println("  1. <credentials-file>.keyfile")
		fmt.Println("  2. keyfile in same directory as credentials")
		os.Exit(1)
	}

	// Parse --keyfile from args
	var keyfilePath string
	args := []string{}
	for i := 2; i < len(os.Args); i++ {
		if os.Args[i] == "--keyfile" && i+1 < len(os.Args) {
			keyfilePath = os.Args[i+1]
			i++
		} else {
			args = append(args, os.Args[i])
		}
	}

	if len(args) < 1 {
		log.Fatal("Missing command")
	}

	cmd := args[0]
	var err error

	switch cmd {
	case "init":
		if len(args) < 2 {
			log.Fatal("Usage: credwrap-server secrets init FILE [--keyfile FILE]")
		}
		err = initCredentials(args[1], keyfilePath)

	case "add":
		if len(args) < 3 {
			log.Fatal("Usage: credwrap-server secrets add FILE KEY [--keyfile FILE]")
		}
		err = addSecret(args[1], args[2], keyfilePath)

	case "list":
		if len(args) < 2 {
			log.Fatal("Usage: credwrap-server secrets list FILE [--keyfile FILE]")
		}
		err = listSecrets(args[1], keyfilePath)

	case "rm", "remove", "delete":
		if len(args) < 3 {
			log.Fatal("Usage: credwrap-server secrets rm FILE KEY [--keyfile FILE]")
		}
		err = removeSecret(args[1], args[2], keyfilePath)

	default:
		log.Fatalf("Unknown secrets command: %s", cmd)
	}

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func handleToolsCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: credwrap-server tools <command> [args]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  add CONFIG NAME PATH [options]")
		fmt.Println("      Copy tool to /usr/local/bin and add to config")
		fmt.Println("")
		fmt.Println("      Options:")
		fmt.Println("        --env VAR     Environment variable for credential (repeatable)")
		fmt.Println("        --symlink     Create symlink instead of copying")
		fmt.Println("        --no-copy     Don't copy; use original path in config")
		fmt.Println("")
		fmt.Println("  list CONFIG     List configured tools")
		fmt.Println("  rm CONFIG NAME  Remove tool from config")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  # Copy binary to /usr/local/bin")
		fmt.Println("  sudo credwrap-server tools add /etc/credwrap/config.yaml gog ~/.local/bin/gog --env GOG_KEYRING_PASSWORD")
		fmt.Println("")
		fmt.Println("  # Symlink for tools that can't be copied (e.g., pnpm scripts)")
		fmt.Println("  sudo credwrap-server tools add /etc/credwrap/config.yaml bird ~/.local/share/pnpm/bird --symlink")
		fmt.Println("")
		fmt.Println("  # Just add to config without copying (when credwrap has access)")
		fmt.Println("  sudo credwrap-server tools add /etc/credwrap/config.yaml gemini /usr/bin/gemini --no-copy")
		os.Exit(1)
	}

	cmd := os.Args[2]
	var err error

	switch cmd {
	case "add":
		if len(os.Args) < 6 {
			log.Fatal("Usage: credwrap-server tools add CONFIG NAME PATH [--env VAR] [--symlink] [--no-copy]")
		}
		configPath := os.Args[3]
		toolName := os.Args[4]
		toolPath := os.Args[5]

		// Parse flags
		var envVars []string
		var opts ToolAddOptions
		for i := 6; i < len(os.Args); i++ {
			switch os.Args[i] {
			case "--env":
				if i+1 < len(os.Args) {
					envVars = append(envVars, os.Args[i+1])
					i++
				}
			case "--symlink":
				opts.Symlink = true
			case "--no-copy":
				opts.NoCopy = true
			}
		}

		err = toolsAdd(configPath, toolName, toolPath, envVars, opts)

	case "list":
		if len(os.Args) < 4 {
			log.Fatal("Usage: credwrap-server tools list CONFIG")
		}
		err = toolsList(os.Args[3])

	case "rm", "remove", "delete":
		if len(os.Args) < 5 {
			log.Fatal("Usage: credwrap-server tools rm CONFIG NAME")
		}
		err = toolsRemove(os.Args[3], os.Args[4])

	default:
		log.Fatalf("Unknown tools command: %s", cmd)
	}

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}
