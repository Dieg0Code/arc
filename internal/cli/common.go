package cli

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/Dieg0Code/arc/internal/config"
	"github.com/Dieg0Code/arc/internal/db"
	"github.com/Dieg0Code/arc/internal/scope"
	"github.com/Dieg0Code/arc/internal/session"
	"github.com/spf13/cobra"
)

// openStore abre el Store local de arc. Falla con un mensaje claro si arc no fue
// inicializado todavía.
func openStore() (db.Store, error) {
	dbPath, err := config.DBPath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dbPath); errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("arc is not initialized on this machine; run 'arc init' first")
	}
	store, err := db.New(db.WithPath(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open arc store: %w", err)
	}
	return store, nil
}

// resolveActiveChat resuelve el chat sobre el que operan los comandos: el flag
// --chat si se pasó, si no la sesión de agente detectada. Devuelve "" si no hay
// ninguna.
func resolveActiveChat(override string) (chatID, source string, err error) {
	if override != "" {
		return override, "", nil
	}
	d, err := session.NewDetector()
	if err != nil {
		return "", "", err
	}
	s, err := d.Detect()
	if err != nil {
		return "", "", err
	}
	if s == nil {
		return "", "", nil
	}
	return s.ChatID, s.Source, nil
}

// shortHash acorta un hash de commit para mostrar.
func shortHash(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

// activeScopeName resuelve el scope activo: flag --scope, si no la variable de
// entorno ARC_SCOPE, si no "" (acceso completo).
func activeScopeName(cmd *cobra.Command) string {
	if v, err := cmd.Flags().GetString("scope"); err == nil && v != "" {
		return v
	}
	return os.Getenv("ARC_SCOPE")
}

// resolveScope traduce el scope activo a la lista de chat ids permitidos.
// Devuelve scoped=false (y allowed=nil) cuando no hay scope activo: en ese caso
// los comandos no filtran nada (comportamiento por defecto).
func resolveScope(cmd *cobra.Command, store db.Store) (allowed []string, scoped bool, err error) {
	name := activeScopeName(cmd)
	if name == "" {
		return nil, false, nil
	}
	scopes, err := config.Scopes()
	if err != nil {
		return nil, false, err
	}
	r, err := scope.New(scope.WithName(name), scope.WithScopes(scopes))
	if err != nil {
		return nil, false, err
	}
	chats, err := store.ListChats()
	if err != nil {
		return nil, false, err
	}
	refs := make([]scope.ChatRef, len(chats))
	for i, c := range chats {
		refs[i] = scope.ChatRef{ID: c.ID, Title: c.Title, Source: c.Source}
	}
	allowed, err = r.AllowedChatIDs(refs)
	if err != nil {
		return nil, false, err
	}
	return allowed, true, nil
}

// inScope indica si chatID está permitido bajo el scope resuelto.
func inScope(allowed []string, chatID string) bool {
	return slices.Contains(allowed, chatID)
}
