package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Dieg0Code/arc/internal/config"
	"github.com/Dieg0Code/arc/internal/scope"
	"github.com/spf13/cobra"
)

// newScopeCmd agrupa los subcomandos de scopes de acceso.
func newScopeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scope",
		Short: "Inspect read-access scopes (defined in ~/.arc/config.toml)",
	}
	cmd.AddCommand(newScopeListCmd(), newScopeShowCmd())
	return cmd
}

// newScopeListCmd crea `arc scope list`: lista los scopes configurados.
func newScopeListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the configured scopes",
		RunE: func(cmd *cobra.Command, args []string) error {
			scopes, err := config.Scopes()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(scopes) == 0 {
				fmt.Fprintln(out, "no scopes configured (define them in ~/.arc/config.toml)")
				return nil
			}
			names := make([]string, 0, len(scopes))
			for n := range scopes {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				s := scopes[n]
				var parts []string
				if len(s.Titles) > 0 {
					parts = append(parts, "titles="+strings.Join(s.Titles, "|"))
				}
				if len(s.Sources) > 0 {
					parts = append(parts, "sources="+strings.Join(s.Sources, "|"))
				}
				if len(s.Chats) > 0 {
					parts = append(parts, fmt.Sprintf("chats=%d", len(s.Chats)))
				}
				fmt.Fprintf(out, "%-16s %s\n", n, strings.Join(parts, "  "))
			}
			return nil
		},
	}
}

// newScopeShowCmd crea `arc scope show <name>`: muestra a qué chats resuelve.
func newScopeShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show which chats a scope resolves to",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			scopes, err := config.Scopes()
			if err != nil {
				return err
			}
			r, err := scope.New(scope.WithName(name), scope.WithScopes(scopes))
			if err != nil {
				return err
			}
			store, err := openStore()
			if err != nil {
				return err
			}
			defer store.Close()

			chats, err := store.ListChats()
			if err != nil {
				return err
			}
			refs := make([]scope.ChatRef, len(chats))
			byID := make(map[string]string, len(chats))
			for i, c := range chats {
				refs[i] = scope.ChatRef{ID: c.ID, Title: c.Title, Source: c.Source}
				label := c.Title
				if label == "" {
					label = "(untitled)"
				}
				byID[c.ID] = fmt.Sprintf("%s · %s", c.Source, label)
			}
			allowed, err := r.AllowedChatIDs(refs)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "scope %q resolves to %d chats:\n", name, len(allowed))
			labels := make([]string, 0, len(allowed))
			for _, id := range allowed {
				labels = append(labels, byID[id])
			}
			sort.Strings(labels)
			for _, l := range labels {
				fmt.Fprintf(out, "  %s\n", l)
			}
			return nil
		},
	}
}
