// Package cli define los comandos de arc sobre cobra.
package cli

import (
	"runtime/debug"

	"github.com/spf13/cobra"
)

// version se inyecta en build time vía -ldflags (GoReleaser). Para instalaciones
// con `go install`, donde no hay ldflags, resolveVersion() cae al módulo.
var version = "dev"

// resolveVersion devuelve la versión inyectada, o la del módulo (build info)
// cuando se instaló con `go install ...@version`.
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

// NewRootCmd construye el comando raíz `arc` con todos sus subcomandos.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "arc",
		Short:         "arc — your agent forgets, arc doesn't",
		Long:          "arc versions agent context the way git versions code.",
		Version:       resolveVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// --scope limita el acceso de lectura a un scope de ~/.arc/config.toml.
	// Vacío = acceso completo. También se puede fijar con ARC_SCOPE.
	root.PersistentFlags().String("scope", "", "limit read access to a scope from config.toml (or set ARC_SCOPE)")

	root.AddCommand(newInitCmd())
	root.AddCommand(newIngestCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newAddCmd())
	root.AddCommand(newCommitCmd())
	root.AddCommand(newLogCmd())
	root.AddCommand(newReadCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newRemoteCmd())
	root.AddCommand(newSyncCmd())
	root.AddCommand(newCloneCmd())
	root.AddCommand(newSkillCmd())
	root.AddCommand(newScopeCmd())

	return root
}
