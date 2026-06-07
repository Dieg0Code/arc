package cli

import (
	"strings"
	"testing"
)

func TestResolveRoles(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		want    []string
		wantErr bool
	}{
		{"empty = conversation + reasoning", "", []string{"user", "assistant", "reasoning"}, false},
		{"all = nil (no filter)", "all", nil, false},
		{"single role", "assistant", []string{"assistant"}, false},
		{"multiple roles", "user,tool", []string{"user", "tool"}, false},
		{"spaces are trimmed", " user , reasoning ", []string{"user", "reasoning"}, false},
		{"unknown role errors", "boss", nil, true},
		{"one bad role in list errors", "user,boss", nil, true},
		{"only commas errors", ",,", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRoles(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %v", tt.flag, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("resolveRoles(%q) = %v, want %v", tt.flag, got, tt.want)
			}
		})
	}
}
