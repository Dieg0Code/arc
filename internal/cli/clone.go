package cli

import (
	"fmt"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/sync"
	"github.com/spf13/cobra"
)

// newCloneCmd crea `nem clone <url>`: clona el store remoto en ~/.nem e importa
// los commits a la DB local.
func newCloneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "clone <url>",
		Short: "Clone a remote nem store and import its commits",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClone(cmd, args[0])
		},
	}
}

func runClone(cmd *cobra.Command, url string) error {
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	if err := sync.Clone(url, dir); err != nil {
		return err
	}

	dbPath, err := config.DBPath()
	if err != nil {
		return err
	}
	store, err := db.New(db.WithPath(dbPath))
	if err != nil {
		return err
	}
	defer store.Close()

	syncer, err := sync.NewSyncer(store, sync.WithDir(dir))
	if err != nil {
		return err
	}
	n, err := syncer.Import()
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cloned at %s · imported %d commits\n", dir, n)
	return nil
}
