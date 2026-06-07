package cli

import (
	"fmt"
	"strings"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/spf13/cobra"
)

// newOutlineCmd crea `nem outline [nodeID]`: imprime el árbol del índice (la
// tabla de contenidos de toda tu memoria) para que el agente decida en qué rama
// bajar. Sin argumento muestra los proyectos; con un nodeID muestra ese subárbol.
func newOutlineCmd() *cobra.Command {
	var (
		depth    int
		chatFlag string
	)
	cmd := &cobra.Command{
		Use:   "outline [nodeID]",
		Short: "Show the index tree (table of contents) to navigate",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			start := ""
			if len(args) == 1 {
				start = args[0]
			}
			return runOutline(cmd, chatFlag, start, depth)
		},
	}
	cmd.Flags().IntVar(&depth, "depth", 2, "how many levels to expand")
	cmd.Flags().StringVar(&chatFlag, "chat", "", "unused placeholder for symmetry")
	_ = cmd.Flags().MarkHidden("chat")
	return cmd
}

func runOutline(cmd *cobra.Command, _ string, start string, depth int) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	allowed, scoped, err := resolveScope(cmd, store)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	var roots []db.Node
	if start == "" {
		roots, err = store.RootNodes()
	} else {
		n, e := store.GetNode(start)
		if e != nil {
			return e
		}
		if n == nil {
			return fmt.Errorf("node %q not found (run 'nem index' first?)", start)
		}
		roots = []db.Node{*n}
	}
	if err != nil {
		return err
	}
	if len(roots) == 0 {
		fmt.Fprintln(out, "empty index — run 'nem index' to build the tree")
		return nil
	}

	for _, r := range roots {
		printNode(cmd, store, r, 0, depth, allowed, scoped)
	}
	return nil
}

// printNode imprime un nodo y recurre por sus hijos hasta `depth`.
func printNode(cmd *cobra.Command, store db.Store, n db.Node, level, depth int, allowed []string, scoped bool) {
	// Filtro de scope: ocultar nodos cuyo chat está fuera del scope.
	if scoped && n.ChatID != "" && !inScope(allowed, n.ChatID) {
		return
	}
	indent := strings.Repeat("  ", level)
	id := n.ID
	line := fmt.Sprintf("%s- [%s] %s", indent, n.Kind, n.Title)
	if s := strings.TrimSpace(n.Summary); s != "" && n.Kind != "project" {
		line += "  — " + s
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s  (%s)\n", line, id)

	if level+1 >= depth {
		return
	}
	children, err := store.ChildNodes(n.ID)
	if err != nil {
		return
	}
	for _, c := range children {
		printNode(cmd, store, c, level+1, depth, allowed, scoped)
	}
}
