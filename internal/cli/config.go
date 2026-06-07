package cli

import (
	"fmt"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/spf13/cobra"
)

// newConfigCmd crea `nem config get/set/list` para los backends de summarize y
// embed (persistidos en ~/.nem/config.toml).
func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Get/set nem settings (summarize/embed backends)",
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "get <key>",
			Short: "Get a config value",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				v, err := config.Get(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), v)
				return nil
			},
		},
		&cobra.Command{
			Use:   "set <key> <value>",
			Short: "Set a config value",
			Args:  cobra.ExactArgs(2),
			RunE: func(cmd *cobra.Command, args []string) error {
				if err := config.Set(args[0], args[1]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s = %s\n", args[0], args[1])
				return nil
			},
		},
		&cobra.Command{
			Use:   "list",
			Short: "List all config keys and their values",
			RunE: func(cmd *cobra.Command, args []string) error {
				out := cmd.OutOrStdout()
				for _, k := range config.Keys() {
					v, err := config.Get(k)
					if err != nil {
						return err
					}
					if v == "" {
						v = "(unset)"
					}
					fmt.Fprintf(out, "%-20s %s\n", k, v)
				}
				return nil
			},
		},
	)
	return cmd
}
