package cli

import (
	"fmt"
	"os"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/sync"
	"github.com/spf13/cobra"
)

// newInitCmd crea el comando `nem init`: prepara ~/.nem/, store/ y la base
// SQLite con el esquema migrado. Es idempotente.
func newInitCmd() *cobra.Command {
	var noSkill bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize the local nem store in ~/.nem",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, noSkill)
		},
	}
	cmd.Flags().BoolVar(&noSkill, "no-skill", false,
		"do not install the nem agent skill into Claude Code / Codex")
	return cmd
}

func runInit(cmd *cobra.Command, noSkill bool) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	chatsDir, err := config.ChatsDir()
	if err != nil {
		return err
	}

	// Crea ~/.nem/store/chats/ (y los padres) si no existen.
	if err := os.MkdirAll(chatsDir, 0o755); err != nil {
		return fmt.Errorf("failed to create store directory: %w", err)
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}

	// Abrir el Store crea nem.db y aplica la migración (idempotente).
	store, err := db.New(db.WithPath(dbPath))
	if err != nil {
		return err
	}
	defer store.Close()

	// Inicializa el repo git + .gitignore para que sync/remote funcionen.
	if err := sync.EnsureRepo(dir); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "nem initialized at %s\n", dir)

	// Instala el agent skill (no fatal: el store ya quedó usable).
	if !noSkill {
		if err := installSkill(cmd); err != nil {
			fmt.Fprintf(cmd.OutOrStdout(), "warning: could not install agent skill: %v\n", err)
		}
	}
	return nil
}
