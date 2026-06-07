package cli

import (
	"fmt"

	"github.com/Dieg0Code/nem/internal/embed"
	"github.com/Dieg0Code/nem/internal/index"
	"github.com/Dieg0Code/nem/internal/summarize"
	"github.com/spf13/cobra"
)

// newIndexCmd crea `nem index`: construye/refresca el árbol navegable
// (project → chat → commit). Los resúmenes de chat usan el backend configurado
// (heurístico por default; ollama/api con --summarize o `nem config`).
func newIndexCmd() *cobra.Command {
	var (
		backend  string
		model    string
		endpoint string
		force    bool
	)
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Build/refresh the navigable index tree (project → chat → commit)",
		Long: "Build/refresh the navigable index tree (project → chat → commit).\n" +
			"Incremental by default: reuses existing summaries/embeddings and only\n" +
			"computes what's new (new chats, new commits). Use --force to redo all.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIndex(cmd, backend, model, endpoint, force)
		},
	}
	cmd.Flags().StringVar(&backend, "summarize", "", "summary backend: heuristic | ollama | api (default: config or heuristic)")
	cmd.Flags().StringVar(&model, "model", "", "model for ollama/api (overrides config)")
	cmd.Flags().StringVar(&endpoint, "endpoint", "", "endpoint for ollama/api (overrides config)")
	cmd.Flags().BoolVar(&force, "force", false, "recompute all summaries/embeddings, ignoring existing work")
	return cmd
}

func runIndex(cmd *cobra.Command, backend, model, endpoint string, force bool) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	opts, err := indexOpts(backend, model, endpoint)
	if err != nil {
		return err
	}
	if force {
		opts = append(opts, index.WithForce(true))
	}
	// Progreso en stderr (stdout queda limpio para el reporte final).
	errw := cmd.ErrOrStderr()
	opts = append(opts, index.WithProgress(func(stage string, done, total int) {
		fmt.Fprintf(errw, "\r%-10s %d/%d ", stage, done, total)
		if done == total {
			fmt.Fprintln(errw)
		}
	}))
	b, err := index.New(store, opts...)
	if err != nil {
		return err
	}
	rep, err := b.Build()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "indexed: %d projects, %d chats, %d commits (%d nodes)\n",
		rep.Projects, rep.Chats, rep.Commits, rep.Nodes)
	if rep.Embedded > 0 {
		fmt.Fprintf(out, "embedded: %d nodes\n", rep.Embedded)
	}
	return nil
}

// indexOpts resuelve el summarizer: el flag --summarize tiene prioridad; si está
// vacío se cae a la config (summarize.FromConfig). "heuristic"/vacío = default.
func indexOpts(backend, model, endpoint string) ([]index.Option, error) {
	var sum summarize.Summarizer
	var err error
	switch backend {
	case "", "config":
		sum, err = summarize.FromConfig()
	case "heuristic":
		sum = nil
	default:
		var o []summarize.Option
		if model != "" {
			o = append(o, summarize.WithModel(model))
		}
		if endpoint != "" {
			o = append(o, summarize.WithEndpoint(endpoint))
		}
		sum, err = summarize.New(backend, o...)
	}
	if err != nil {
		return nil, err
	}
	var opts []index.Option
	if sum != nil {
		opts = append(opts, index.WithSummarizer(sum))
	}
	// Embeddings: capa opcional, según config [embed] (apagada si no se configuró).
	if emb, e := embed.FromConfig(); e == nil && emb != nil {
		opts = append(opts, index.WithEmbedder(emb))
	}
	return opts, nil
}
