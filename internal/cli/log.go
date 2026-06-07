package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newLogCmd crea `nem log`: lista los commits del chat activo.
func newLogCmd() *cobra.Command {
	var (
		graph    bool
		chatFlag string
	)
	cmd := &cobra.Command{
		Use:   "log",
		Short: "List the commits of the active chat",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLog(cmd, chatFlag, graph)
		},
	}
	cmd.Flags().BoolVar(&graph, "graph", false, "show a simple graph")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "chat id (default: detected session)")
	return cmd
}

func runLog(cmd *cobra.Command, chatFlag string, graph bool) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	chatID, _, err := resolveActiveChat(chatFlag)
	if err != nil {
		return err
	}
	if chatID == "" {
		return errors.New("no active session detected; use --chat <id>")
	}

	allowed, scoped, err := resolveScope(cmd, store)
	if err != nil {
		return err
	}
	if scoped && !inScope(allowed, chatID) {
		fmt.Fprintf(cmd.OutOrStdout(), "(active chat is out of scope %q)\n", activeScopeName(cmd))
		return nil
	}

	commits, err := store.ListCommits(chatID)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(commits) == 0 {
		fmt.Fprintln(out, "no commits yet (use 'nem add' + 'nem commit')")
		return nil
	}

	prefix := ""
	if graph {
		prefix = "* "
	}
	for _, c := range commits {
		date := time.Unix(c.CreatedAt, 0).Format("2006-01-02 15:04")
		fmt.Fprintf(out, "%s%s  %s  %s\n", prefix, shortHash(c.Hash), date, c.Message)
	}
	return nil
}
