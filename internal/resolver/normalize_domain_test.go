package resolver_test

import (
	"testing"

	"github.com/kudanilll/favget/internal/resolver"
)

// TestNormalizeDomain covers common and edge cases in a table-driven style.
// The goal is to ensure all user input variants are normalized consistently
// and invalid inputs are rejected.
func TestNormalizeDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		in        string
		want      string
		wantError bool
	}{
		// happy paths
		{"plain", "github.com", "github.com", false},
		{"lowercasing", "GITHUB.COM", "github.com", false},
		{"strip-http", "http://github.com", "github.com", false},
		{"strip-https", "https://github.com", "github.com", false},
		{"strip-www", "www.github.com", "github.com", false},
		{"trim-spaces", "  github.com  ", "github.com", false},

		// invalids
		{"empty", "", "", true},
		{"has-path", "github.com/logo.png", "", true},
		{"has-query", "github.com?x=1", "", true},
		{"has-fragment", "github.com#x", "", true},
	}

	for _, tt := range tests {
		tt := tt // capture
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolver.NormalizeDomain(tt.in)
			if tt.wantError {
				if err == nil {
					t.Fatalf("NormalizeDomain(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeDomain(%q) unexpected error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeDomain(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
