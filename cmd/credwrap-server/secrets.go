package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"filippo.io/age"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// getPassword gets the encryption password from keyfile or prompt
// Looks for keyfile in order:
// 1. Explicit keyfile path (if provided)
// 2. <credsPath>.keyfile (e.g., credentials.enc.keyfile)
// 3. keyfile in same directory as credentials
// 4. Interactive prompt
func getPassword(credsPath, keyfilePath string) (string, error) {
	// Try explicit keyfile
	if keyfilePath != "" {
		data, err := os.ReadFile(keyfilePath)
		if err != nil {
			return "", fmt.Errorf("reading keyfile: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Try <credsPath>.keyfile
	autoKeyfile := credsPath + ".keyfile"
	if data, err := os.ReadFile(autoKeyfile); err == nil {
		fmt.Printf("Using keyfile: %s\n", autoKeyfile)
		return strings.TrimSpace(string(data)), nil
	}

	// Try keyfile in same directory
	dirKeyfile := filepath.Join(filepath.Dir(credsPath), "keyfile")
	if data, err := os.ReadFile(dirKeyfile); err == nil {
		fmt.Printf("Using keyfile: %s\n", dirKeyfile)
		return strings.TrimSpace(string(data)), nil
	}

	// Interactive prompt
	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()
	return string(password), nil
}

// getNewPassword gets a new password with confirmation
func getNewPassword(keyfilePath string) (string, error) {
	// If keyfile provided, use it
	if keyfilePath != "" {
		data, err := os.ReadFile(keyfilePath)
		if err != nil {
			return "", fmt.Errorf("reading keyfile: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}

	// Interactive with confirmation
	fmt.Print("Enter encryption password: ")
	password, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("reading password: %w", err)
	}
	fmt.Println()

	fmt.Print("Confirm password: ")
	confirm, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", fmt.Errorf("reading confirmation: %w", err)
	}
	fmt.Println()

	if string(password) != string(confirm) {
		return "", fmt.Errorf("passwords don't match")
	}

	return string(password), nil
}

// addSecret adds a secret to an encrypted credentials file without
// ever writing plaintext to disk
func addSecret(credsPath, secretName, keyfilePath string) error {
	// Check if file exists
	isNew := false
	if _, err := os.Stat(credsPath); os.IsNotExist(err) {
		isNew = true
	}

	// Get password
	var password string
	var err error
	if isNew {
		password, err = getNewPassword(keyfilePath)
	} else {
		password, err = getPassword(credsPath, keyfilePath)
	}
	if err != nil {
		return err
	}

	// Load existing credentials or start fresh
	creds := make(map[string]string)
	if !isNew {
		// Decrypt existing file
		encData, err := os.ReadFile(credsPath)
		if err != nil {
			return fmt.Errorf("reading file: %w", err)
		}

		identity, err := age.NewScryptIdentity(password)
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
	recipient, err := age.NewScryptRecipient(password)
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
func listSecrets(credsPath, keyfilePath string) error {
	encData, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	password, err := getPassword(credsPath, keyfilePath)
	if err != nil {
		return err
	}

	identity, err := age.NewScryptIdentity(password)
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
func removeSecret(credsPath, secretName, keyfilePath string) error {
	encData, err := os.ReadFile(credsPath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	password, err := getPassword(credsPath, keyfilePath)
	if err != nil {
		return err
	}

	identity, err := age.NewScryptIdentity(password)
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
	recipient, _ := age.NewScryptRecipient(password)

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
func initCredentials(credsPath, keyfilePath string) error {
	if _, err := os.Stat(credsPath); err == nil {
		fmt.Print("File exists. Overwrite? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(response)), "y") {
			return fmt.Errorf("aborted")
		}
	}

	password, err := getNewPassword(keyfilePath)
	if err != nil {
		return err
	}

	// Create empty credentials
	creds := map[string]string{}
	plaintext, _ := yaml.Marshal(creds)

	recipient, _ := age.NewScryptRecipient(password)
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
