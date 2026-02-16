package cli

import "testing"

func TestMaskToken_Long(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ghp_1234567890abcdef", "ghp_...cdef"},
		{"github_pat_abcdefghijklmnop", "gith...mnop"},
	}
	for _, tt := range tests {
		got := maskToken(tt.input)
		if got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskToken_Short(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"short", "****"},
		{"123456789012", "****"},
		{"", "****"},
	}
	for _, tt := range tests {
		got := maskToken(tt.input)
		if got != tt.want {
			t.Errorf("maskToken(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestMaskToken_Boundary(t *testing.T) {
	// 13 chars — should show first 4 and last 4
	got := maskToken("1234567890abc")
	if got != "1234...0abc" {
		t.Errorf("maskToken(13 chars) = %q, want %q", got, "1234...0abc")
	}
}
