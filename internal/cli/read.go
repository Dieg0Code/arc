package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Dieg0Code/arc/internal/db"
	"github.com/Dieg0Code/arc/internal/output"
	"github.com/spf13/cobra"
)

// newReadCmd crea `arc read HEAD|<hash>`: muestra el snapshot de un commit.
func newReadCmd() *cobra.Command {
	var (
		format   string
		chatFlag string
	)
	cmd := &cobra.Command{
		Use:   "read <HEAD|hash>",
		Short: "Show the contents of a commit",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRead(cmd, chatFlag, args[0], format)
		},
	}
	cmd.Flags().StringVar(&format, "format", output.FormatMarkdown, "llm | json | markdown")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "chat id (for HEAD; default: detected session)")
	return cmd
}

func runRead(cmd *cobra.Command, chatFlag, ref, format string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	commit, err := resolveCommit(store, chatFlag, ref)
	if err != nil {
		return err
	}
	if commit == nil {
		return fmt.Errorf("commit %q not found", ref)
	}

	// Enforcement de scope: un commit fuera del scope activo no es legible.
	allowed, scoped, err := resolveScope(cmd, store)
	if err != nil {
		return err
	}
	if scoped && !inScope(allowed, commit.ChatID) {
		return fmt.Errorf("commit %q not found in scope %q", ref, activeScopeName(cmd))
	}

	snap, err := output.ParseSnapshot(commit.Snapshot)
	if err != nil {
		return err
	}

	chat, err := store.GetChat(commit.ChatID)
	if err != nil {
		return err
	}
	doc := output.Doc{
		Date:     time.Unix(commit.CreatedAt, 0),
		Messages: snap,
		Commit:   commit,
	}
	if chat != nil {
		doc.Title = chat.Title
		doc.Source = chat.Source
	}

	rendered, err := output.Render(doc, format)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), rendered)
	return nil
}

// resolveCommit resuelve "HEAD" (último commit del chat activo) o un hash/prefijo.
func resolveCommit(store db.Store, chatFlag, ref string) (*db.Commit, error) {
	if strings.EqualFold(ref, "HEAD") {
		chatID, _, err := resolveActiveChat(chatFlag)
		if err != nil {
			return nil, err
		}
		if chatID == "" {
			return nil, errors.New("no active session detected for HEAD; use --chat <id>")
		}
		return store.HeadCommit(chatID)
	}
	return store.GetCommit(ref)
}
