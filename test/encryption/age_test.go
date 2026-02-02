package encryption_test

import (
    "bytes"
    "io"
    "testing"

    "filippo.io/age"
    "gopkg.in/yaml.v3"
)

func TestAgeEncryption(t *testing.T) {
    // Original credentials
    creds := map[string]string{
        "test-secret": "hello-encrypted-world",
        "another-key": "super-secret-value",
    }
    
    // Serialize to YAML
    plaintext, err := yaml.Marshal(creds)
    if err != nil {
        t.Fatalf("marshal: %v", err)
    }
    
    // Encrypt with password
    password := "testpass123"
    recipient, err := age.NewScryptRecipient(password)
    if err != nil {
        t.Fatalf("new recipient: %v", err)
    }
    
    var encrypted bytes.Buffer
    writer, err := age.Encrypt(&encrypted, recipient)
    if err != nil {
        t.Fatalf("encrypt: %v", err)
    }
    writer.Write(plaintext)
    writer.Close()
    
    t.Logf("Encrypted %d bytes -> %d bytes", len(plaintext), encrypted.Len())
    
    // Decrypt
    identity, err := age.NewScryptIdentity(password)
    if err != nil {
        t.Fatalf("new identity: %v", err)
    }
    
    reader, err := age.Decrypt(bytes.NewReader(encrypted.Bytes()), identity)
    if err != nil {
        t.Fatalf("decrypt: %v", err)
    }
    
    decrypted, err := io.ReadAll(reader)
    if err != nil {
        t.Fatalf("read: %v", err)
    }
    
    // Parse back to map
    var result map[string]string
    if err := yaml.Unmarshal(decrypted, &result); err != nil {
        t.Fatalf("unmarshal: %v", err)
    }
    
    // Verify
    if result["test-secret"] != "hello-encrypted-world" {
        t.Errorf("wrong value: %v", result)
    }
    if result["another-key"] != "super-secret-value" {
        t.Errorf("wrong value: %v", result)
    }
    
    t.Log("✓ Age encryption working!")
}
