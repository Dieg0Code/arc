package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/output"
	"github.com/spf13/cobra"
)

// chatReadLimit acota cuántos mensajes de conversación se muestran al leer un
// nodo de chat (drill-down acotado para no quemar tokens).
const chatReadLimit = 40

// newReadCmd crea `nem read <ref>`: muestra contenido. ref puede ser HEAD, un
// hash de commit, `commit:<hash>` o `chat:<id>` (nodos del árbol).
func newReadCmd() *cobra.Command {
	var (
		format   string
		chatFlag string
	)
	cmd := &cobra.Command{
		Use:   "read <HEAD|hash|commit:hash|chat:id>",
		Short: "Show the contents of a commit or chat node",
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

	allowed, scoped, err := resolveScope(cmd, store)
	if err != nil {
		return err
	}

	// Nodo de chat: drill-down a los últimos mensajes de conversación.
	if strings.HasPrefix(ref, "chat:") {
		chatID := strings.TrimPrefix(ref, "chat:")
		if scoped && !inScope(allowed, chatID) {
			return fmt.Errorf("chat %q not found in scope %q", chatID, activeScopeName(cmd))
		}
		return readChat(cmd, store, chatID, format)
	}

	// Commit: HEAD, hash, o commit:<hash>.
	commit, err := resolveCommit(store, chatFlag, strings.TrimPrefix(ref, "commit:"))
	if err != nil {
		return err
	}
	if commit == nil {
		return fmt.Errorf("commit %q not found", ref)
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

// readChat renderiza los últimos mensajes de conversación de un chat.
func readChat(cmd *cobra.Command, store db.Store, chatID, format string) error {
	chat, err := store.GetChat(chatID)
	if err != nil {
		return err
	}
	if chat == nil {
		return fmt.Errorf("chat %q not found", chatID)
	}
	msgs, err := store.LastMessages(chatID, chatReadLimit, []string{"user", "assistant", "reasoning"})
	if err != nil {
		return err
	}
	snap := make([]output.SnapMessage, 0, len(msgs))
	for _, m := range msgs {
		snap = append(snap, output.SnapMessage{
			Role: m.Role, Content: m.Content, Timestamp: m.Timestamp, Seq: m.Seq,
		})
	}
	doc := output.Doc{
		Title:    chat.Title,
		Source:   chat.Source,
		Date:     time.Unix(chat.CreatedAt, 0),
		Messages: snap,
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
