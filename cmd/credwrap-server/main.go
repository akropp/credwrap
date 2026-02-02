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
  credwrap-server secrets init FILE    Create new encrypted credentials file
  credwrap-server secrets add FILE KEY Add a secret (never touches disk in plaintext)
  credwrap-server secrets list FILE    List secret names
  credwrap-server secrets rm FILE KEY  Remove a secret

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
		fmt.Println("Usage: credwrap-server secrets <command> [args]")
		fmt.Println("")
		fmt.Println("Commands:")
		fmt.Println("  init FILE        Create new encrypted credentials file")
		fmt.Println("  add FILE KEY     Add/update a secret")
		fmt.Println("  list FILE        List secret names (not values)")
		fmt.Println("  rm FILE KEY      Remove a secret")
		os.Exit(1)
	}

	cmd := os.Args[2]
	var err error

	switch cmd {
	case "init":
		if len(os.Args) < 4 {
			log.Fatal("Usage: credwrap-server secrets init FILE")
		}
		err = initCredentials(os.Args[3])

	case "add":
		if len(os.Args) < 5 {
			log.Fatal("Usage: credwrap-server secrets add FILE KEY")
		}
		err = addSecret(os.Args[3], os.Args[4])

	case "list":
		if len(os.Args) < 4 {
			log.Fatal("Usage: credwrap-server secrets list FILE")
		}
		err = listSecrets(os.Args[3])

	case "rm", "remove", "delete":
		if len(os.Args) < 5 {
			log.Fatal("Usage: credwrap-server secrets rm FILE KEY")
		}
		err = removeSecret(os.Args[3], os.Args[4])

	default:
		log.Fatalf("Unknown secrets command: %s", cmd)
	}

	if err != nil {
		log.Fatalf("Error: %v", err)
	}
}
