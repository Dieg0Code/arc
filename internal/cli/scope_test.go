package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func cmdWithScopeFlag(flagVal string) *cobra.Command {
	cmd := &cobra.Command{Use: "x"}
	cmd.Flags().String("scope", "", "")
	if flagVal != "" {
		_ = cmd.Flags().Set("scope", flagVal)
	}
	return cmd
}

func TestActiveScopeName_Precedence(t *testing.T) {
	tests := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"flag only", "ataxx", "", "ataxx"},
		{"env only", "", "teaching", "teaching"},
		{"flag beats env", "ataxx", "teaching", "ataxx"},
		{"neither", "", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NEM_SCOPE", tt.env)
			got := activeScopeName(cmdWithScopeFlag(tt.flag))
			if got != tt.want {
				t.Errorf("activeScopeName() = %q, want %q", got, tt.want)
			}
		})
	}
}
