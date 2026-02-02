package server

import (
	"testing"
)

func TestExtractIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"127.0.0.1:12345", "127.0.0.1"},
		{"192.168.1.1:80", "192.168.1.1"},
		{"[::1]:8080", "::1"},
		{"100.64.1.100:54321", "100.64.1.100"},
		{"invalid", "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractIP(tt.input)
			if result != tt.expected {
				t.Errorf("extractIP(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestMatchIP(t *testing.T) {
	tests := []struct {
		clientIP string
		allowed  string
		expected bool
	}{
		// Exact matches
		{"127.0.0.1", "127.0.0.1", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.1.1", "192.168.1.2", false},

		// CIDR ranges
		{"192.168.1.50", "192.168.1.0/24", true},
		{"192.168.2.50", "192.168.1.0/24", false},
		{"100.64.1.100", "100.64.0.0/10", true},
		{"192.168.1.1", "100.64.0.0/10", false},
		{"10.0.0.1", "10.0.0.0/8", true},

		// Localhost
		{"127.0.0.1", "127.0.0.0/8", true},
		{"127.0.0.1", "127.0.0.1/32", true},
	}

	for _, tt := range tests {
		name := tt.clientIP + "_" + tt.allowed
		t.Run(name, func(t *testing.T) {
			result := matchIP(tt.clientIP, tt.allowed)
			if result != tt.expected {
				t.Errorf("matchIP(%q, %q) = %v, want %v", tt.clientIP, tt.allowed, result, tt.expected)
			}
		})
	}
}
