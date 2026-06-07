package cli

import (
	nemmcp "github.com/Dieg0Code/nem/internal/mcp"
	"github.com/spf13/cobra"
)

// newMCPCmd crea `nem mcp`: corre nem como servidor MCP por stdio. No imprime
// nada a stdout (stdout es el transporte JSON-RPC del protocolo).
func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Run nem as an MCP server (stdio) so agents use it as tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()
			return nemmcp.Serve(cmd.Context(), store, resolveVersion())
		},
	}
}
