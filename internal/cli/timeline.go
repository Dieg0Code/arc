package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
)

// newTimelineCmd crea `nem timeline <project|chatID>`: muestra la evolución
// temporal de las decisiones (commits) de un proyecto o chat, del más viejo al
// más nuevo, marcando el último como actual. Es la idea temporal de Zep, barata:
// el agente ve cómo cambió una decisión en el tiempo.
func newTimelineCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "timeline <project|chatID>",
		Short: "Show how a project's/chat's decisions evolved over time",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTimeline(cmd, args[0])
		},
	}
	return cmd
}

func runTimeline(cmd *cobra.Command, target string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	// Resolver target → chatIDs: primero como nombre de proyecto (título de
	// chat); si no matchea nada, tratarlo como chatID.
	chats, err := store.ListChats()
	if err != nil {
		return err
	}
	var chatIDs []string
	for _, c := range chats {
		if c.Title == target {
			chatIDs = append(chatIDs, c.ID)
		}
	}
	if len(chatIDs) == 0 {
		chatIDs = []string{target} // tratar como chatID
	}

	nodes, err := store.CommitNodes(chatIDs)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(nodes) == 0 {
		fmt.Fprintf(out, "no commits for %q (nothing committed yet, or run 'nem index')\n", target)
		return nil
	}

	fmt.Fprintf(out, "timeline of %q — %d commits (oldest → newest):\n\n", target, len(nodes))
	for i, n := range nodes {
		date := time.Unix(n.CreatedAt, 0).Format("2006-01-02 15:04")
		marker := ""
		if i == len(nodes)-1 {
			marker = "  ← current"
		}
		fmt.Fprintf(out, "%s  %s  %s%s\n", date, shortHash(n.CommitHash), n.Title, marker)
	}
	return nil
}
