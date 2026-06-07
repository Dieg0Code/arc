package cli

import (
	"errors"
	"fmt"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/spf13/cobra"
)

// newAddCmd crea `nem add`: marca mensajes para el próximo commit (staging).
func newAddCmd() *cobra.Command {
	var (
		lastN    int
		fromID   string
		toID     string
		chatFlag string
		role     string
	)
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Stage messages for the next commit",
		Long:  "nem add -L <n>  ·  nem add --from <msgID> --to <msgID>",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAdd(cmd, chatFlag, lastN, fromID, toID, role)
		},
	}
	cmd.Flags().IntVarP(&lastN, "last", "L", 0, "stage the last N messages (of the selected roles)")
	cmd.Flags().StringVar(&fromID, "from", "", "id of the first message in the range")
	cmd.Flags().StringVar(&toID, "to", "", "id of the last message in the range")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "chat id (default: detected session)")
	cmd.Flags().StringVar(&role, "role", "", "roles to stage, comma-separated (default: conversation + reasoning; 'all' = every role, incl. tool)")
	return cmd
}

func runAdd(cmd *cobra.Command, chatFlag string, lastN int, fromID, toID, role string) error {
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

	roles, err := resolveRoles(role)
	if err != nil {
		return err
	}

	var msgs []db.Message
	switch {
	case lastN > 0:
		msgs, err = store.LastMessages(chatID, lastN, roles)
	case fromID != "" && toID != "":
		msgs, err = rangeMessages(store, chatID, fromID, toID, roles)
	default:
		return errors.New("specify -L <n> or --from <id> --to <id>")
	}
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return errors.New("no messages to stage (did you ingest the chat?)")
	}

	n, err := store.StageMessages(chatID, msgs)
	if err != nil {
		return err
	}
	total, err := store.CountStaged(chatID)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "staged %d new messages (total staged: %d)\n", n, total)
	return nil
}

// rangeMessages resuelve --from/--to (ids de mensaje) a un rango por Seq,
// filtrando por roles.
func rangeMessages(store db.Store, chatID, fromID, toID string, roles []string) ([]db.Message, error) {
	from, err := store.MessageByID(chatID, fromID)
	if err != nil {
		return nil, err
	}
	to, err := store.MessageByID(chatID, toID)
	if err != nil {
		return nil, err
	}
	if from == nil || to == nil {
		return nil, fmt.Errorf("--from and/or --to not found in chat %s", chatID)
	}
	return store.MessagesBySeqRange(chatID, from.Seq, to.Seq, roles)
}
