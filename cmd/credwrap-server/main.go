// credwrap-server is the credential wrapper daemon.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/openclaw/credwrap/internal/config"
	"github.com/openclaw/credwrap/internal/server"
	"golang.org/x/term"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	credsPath := flag.String("credentials", "credentials.yaml", "Path to credentials file")
	encrypted := flag.Bool("encrypted", false, "Credentials file is age-encrypted")
	flag.Parse()

	// Load config
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Load credentials
	var creds map[string]string
	if *encrypted {
		// Prompt for password
		fmt.Print("Enter decryption password: ")
		password, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			log.Fatalf("Failed to read password: %v", err)
		}
		fmt.Println()

		creds, err = config.LoadCredentialsEncrypted(*credsPath, string(password))
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
