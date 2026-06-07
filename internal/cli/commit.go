package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/output"
	"github.com/spf13/cobra"
)

// newCommitCmd crea `nem commit -m`: congela los mensajes staged en un commit
// inmutable (copia el texto en un snapshot).
func newCommitCmd() *cobra.Command {
	var (
		message  string
		chatFlag string
	)
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Create an immutable commit from the staged messages",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCommit(cmd, chatFlag, message)
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "commit message (required)")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "chat id (default: detected session)")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func runCommit(cmd *cobra.Command, chatFlag, message string) error {
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

	staged, err := store.StagedMessages(chatID)
	if err != nil {
		return err
	}
	if len(staged) == 0 {
		return errors.New("nothing staged; run 'nem add -L <n>' first")
	}

	snapshot, err := output.BuildSnapshot(staged)
	if err != nil {
		return err
	}

	commit := &db.Commit{
		Hash:      commitHash(chatID, message, snapshot),
		ChatID:    chatID,
		Branch:    "main",
		Message:   message,
		MsgFrom:   staged[0].ID,
		MsgTo:     staged[len(staged)-1].ID,
		Snapshot:  snapshot,
		CreatedAt: time.Now().Unix(),
	}
	if err := store.CreateCommit(commit); err != nil {
		return err
	}
	if err := store.ClearStaging(chatID); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "commit %s: %q (%d messages)\n",
		shortHash(commit.Hash), message, len(staged))
	return nil
}

// commitHash deriva un hash estable del contenido del commit (chat + mensaje +
// snapshot). El snapshot es inmutable, así que el hash identifica el contenido.
func commitHash(chatID, message, snapshot string) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\n%s\n%d\n%s", chatID, message, time.Now().UnixNano(), snapshot)
	return hex.EncodeToString(h.Sum(nil))
}
