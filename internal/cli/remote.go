package cli

import (
	"fmt"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/Dieg0Code/nem/internal/sync"
	"github.com/spf13/cobra"
)

// newRemoteCmd crea `nem remote` (lista) y `nem remote add <name> <url>`.
func newRemoteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage the sync remote",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			out, err := sync.RemoteList(dir)
			if err != nil {
				return err
			}
			if out == "" {
				fmt.Fprintln(cmd.OutOrStdout(), "no remotes (use 'nem remote add origin <url>')")
				return nil
			}
			fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	// -v se acepta por compatibilidad; el comando lista igual.
	cmd.Flags().BoolP("verbose", "v", false, "list the remotes")
	cmd.AddCommand(newRemoteAddCmd())
	return cmd
}

func newRemoteAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <url>",
		Short: "Add or update a remote",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := config.Dir()
			if err != nil {
				return err
			}
			if err := sync.RemoteAdd(dir, args[0], args[1]); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "remote %s → %s\n", args[0], args[1])
			return nil
		},
	}
}
