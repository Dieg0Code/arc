package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newStatusCmd crea `nem status`: muestra la sesión activa y su estado.
func newStatusCmd() *cobra.Command {
	var chatFlag string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the active session and uncommitted messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd, chatFlag)
		},
	}
	cmd.Flags().StringVar(&chatFlag, "chat", "", "chat id (default: detected session)")
	return cmd
}

func runStatus(cmd *cobra.Command, chatFlag string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	out := cmd.OutOrStdout()

	chatID, source, err := resolveActiveChat(chatFlag)
	if err != nil {
		return err
	}
	if chatID == "" {
		fmt.Fprintln(out, "No active session detected (Codex/Claude).")
		fmt.Fprintln(out, "Open a session with your agent, or use 'nem status --chat <id>'.")
		return nil
	}

	chat, err := store.GetChat(chatID)
	if err != nil {
		return err
	}
	if chat == nil {
		fmt.Fprintf(out, "Active session detected (%s): %s\n", source, chatID)
		fmt.Fprintln(out, "Not ingested yet. Run 'nem ingest' to pull it in.")
		return nil
	}

	count, err := store.CountMessages(chatID)
	if err != nil {
		return err
	}
	staged, err := store.CountStaged(chatID)
	if err != nil {
		return err
	}
	head, err := store.HeadCommit(chatID)
	if err != nil {
		return err
	}

	title := chat.Title
	if title == "" {
		title = "(untitled)"
	}
	fmt.Fprintf(out, "Active session: %s · %s\n", chat.Source, title)
	fmt.Fprintf(out, "  chat:     %s\n", chat.ID)
	fmt.Fprintf(out, "  messages: %d\n", count)
	fmt.Fprintf(out, "  staged:   %d\n", staged)
	if head != nil {
		fmt.Fprintf(out, "  HEAD:     %s  %q\n", shortHash(head.Hash), head.Message)
	} else {
		fmt.Fprintf(out, "  HEAD:     (no commits)\n")
	}

	if activeScopeName(cmd) != "" {
		allowed, scoped, err := resolveScope(cmd, store)
		if err != nil {
			return err
		}
		if scoped {
			state := "active chat in scope"
			if !inScope(allowed, chat.ID) {
				state = "active chat OUT of scope"
			}
			fmt.Fprintf(out, "  scope:    %s (%s)\n", activeScopeName(cmd), state)
		}
	}
	return nil
}
