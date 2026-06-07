package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/embed"
	"github.com/spf13/cobra"
)

// newAnnotateCmd crea `nem annotate <nodeID> -m`: reescribe el resumen de un nodo
// del índice (project/chat/commit). Es la capa MUTABLE: el agente o el humano
// corrige un resumen flojo o equivocado, y queda "pinned" — un re-index no lo
// vuelve a generar. Los commits siguen siendo inmutables; solo cambia su resumen
// navegable.
func newAnnotateCmd() *cobra.Command {
	var summary string
	cmd := &cobra.Command{
		Use:   "annotate <nodeID>",
		Short: "Rewrite a node's summary (agent/human-authored; survives re-index)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAnnotate(cmd, args[0], summary)
		},
	}
	cmd.Flags().StringVarP(&summary, "message", "m", "", "the summary to set (required)")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func runAnnotate(cmd *cobra.Command, nodeID, summary string) error {
	if strings.TrimSpace(summary) == "" {
		return errors.New("summary is required (use -m)")
	}
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	node, err := store.GetNode(nodeID)
	if err != nil {
		return err
	}
	if node == nil {
		return fmt.Errorf("node %q not found (run 'nem index'? ids look like project:foo, chat:id, commit:hash)", nodeID)
	}
	if err := store.SetNodeSummary(nodeID, summary); err != nil {
		return err
	}
	// Si los embeddings están activos, re-embeber solo este nodo para que la capa
	// semántica refleje el nuevo resumen. Si el backend no está, no pasa nada.
	reembedNode(store, nodeID, node.Title, summary)

	fmt.Fprintf(cmd.OutOrStdout(), "annotated %s (pinned; 'nem index' won't overwrite it)\n", nodeID)
	return nil
}

// reembedNode recomputa el embedding de un solo nodo (best-effort).
func reembedNode(store db.Store, id, title, summary string) {
	emb, err := embed.FromConfig()
	if err != nil || emb == nil {
		return
	}
	vecs, err := emb.Embed(context.Background(), []string{strings.TrimSpace(title + "\n" + summary)})
	if err != nil || len(vecs) != 1 || len(vecs[0]) == 0 {
		return
	}
	_, _ = store.UpsertEmbeddings([]db.Embedding{{NodeID: id, Dim: len(vecs[0]), Vec: embed.Encode(vecs[0])}})
}
