package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"syscall"

	"filippo.io/age"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// addSecret adds a secret to an encrypted credentials file without
// ever writing plaintext to disk
func addSecret(credsPath, secretName string) error {
	// Check if file exists
	isNew := false
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		isNew = true
	}

	// Get password
	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()

	// Load existing credentials or start fresh
	creds := make(map[string]string)
	if !isNew {
		// Decrypt existing file
		encData, err := os.ReadFile(credsPath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		identity, err := age.NewScryptIdentity(string(password))
		if err != nil {
			return fmt.Errorf("creating identity: %w", err)
		}

		reader, err := age.Decrypt(bytes.NewReader(encData), identity)
		if err != nil {
			return fmt.Errorf("decrypting (wrong password?): %w", err)
		}

		data, err := io.ReadAll(reader)
		if err != nil {
			return fmt.Errorf("reading decrypted data: %w", err)
		}

		if err := yaml.Unmarshal(data, &creds); err != nil {
			return fmt.Errorf("parsing credentials: %w", err)
		}
	} else {
		// New file - confirm password
		fmt.Print("Confirm password: ")
		confirm, err := term.ReadPassword(int(syscall.Stdin))
		if err != nil {
			return fmt.Errorf("reading confirmation: %w", err)
		}
		fmt.Println()

		if string(password) != string(confirm) {
			return fmt.Errorf("passwords don't match")
		}
	}

	// Get the secret value
	fmt.Printf("Enter value for '%s': ", secretName)
	secretValue, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading secret: %w", err)
	}
	fmt.Println()

	// Add/update the secret
	creds[secretName] = string(secretValue)

	// Serialize to YAML
	plaintext, err := yaml.Marshal(creds)
	if err != nil {
		return fmt.Errorf("serializing: %w", err)
	}

	// Encrypt
	recipient, err := age.NewScryptRecipient(string(password))
	if err != nil {
		return fmt.Errorf("creating recipient: %w", err)
	}

	var encrypted bytes.Buffer
	writer, err := age.Encrypt(&encrypted, recipient)
	if err != nil {
		return fmt.Errorf("creating encryptor: %w", err)
	}
	writer.Write(plaintext)
	writer.Close()

	// Write to file
	if err := os.WriteFile(credsPath, encrypted.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}

	fmt.Printf("✓ Secret '%s' added to %s\n", secretName, credsPath)
	return nil
}

// listSecrets lists secret names (not values) from an encrypted file
func listSecrets(credsPath string) error {
	encData, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()

	identity, err := age.NewScryptIdentity(string(password))
	if err != nil {
		return fmt.Errorf("creating identity: %w", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(encData), identity)
	if err != nil {
		return fmt.Errorf("decrypting (wrong password?): %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}

	var creds map[string]string
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("parsing: %w", err)
	}

	fmt.Printf("Secrets in %s:\n", credsPath)
	for name := range creds {
		fmt.Printf("  - %s\n", name)
	}
	return nil
}

// removeSecret removes a secret from an encrypted file
func removeSecret(credsPath, secretName string) error {
	encData, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()

	identity, err := age.NewScryptIdentity(string(password))
	if err != nil {
		return fmt.Errorf("creating identity: %w", err)
	}

	reader, err := age.Decrypt(bytes.NewReader(encData), identity)
	if err != nil {
		return fmt.Errorf("decrypting (wrong password?): %w", err)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("reading: %w", err)
	}

	var creds map[string]string
	if err := yaml.Unmarshal(data, &creds); err != nil {
		return fmt.Errorf("parsing: %w", err)
	}

	if _, exists := creds[secretName]; !exists {
		return fmt.Errorf("secret '%s' not found", secretName)
	}

	delete(creds, secretName)

	// Re-encrypt
	plaintext, _ := yaml.Marshal(creds)
	recipient, _ := age.NewScryptRecipient(string(password))

	var encrypted bytes.Buffer
	writer, _ := age.Encrypt(&encrypted, recipient)
	writer.Write(plaintext)
	writer.Close()

	if err := os.WriteFile(credsPath, encrypted.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	fmt.Printf("✓ Secret '%s' removed from %s\n", secretName, credsPath)
	return nil
}

// initCredentials creates a new encrypted credentials file
func initCredentials(credsPath string) error {
	if _, err := os.Stat(credsPath); err == nil {
		fmt.Print("File exists. Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(response)), "y") {
			return fmt.Errorf("aborted")
		}
	}

	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("reading confirmation: %w", err)
	}
	fmt.Println()

	if string(password) != string(confirm) {
		return fmt.Errorf("passwords don't match")
	}

	// Create empty credentials
	creds := map[string]string{}
	plaintext, _ := yaml.Marshal(creds)

	recipient, _ := age.NewScryptRecipient(string(password))
	var encrypted bytes.Buffer
	writer, _ := age.Encrypt(&encrypted, recipient)
	writer.Write(plaintext)
	writer.Close()

	if err := os.WriteFile(credsPath, encrypted.Bytes(), 0600); err != nil {
		return fmt.Errorf("writing: %w", err)
	}

	fmt.Printf("✓ Created encrypted credentials file: %s\n", credsPath)
	return nil
}
