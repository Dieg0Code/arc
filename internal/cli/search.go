package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/output"
	"github.com/spf13/cobra"
)

// snippetLen acota el contenido mostrado por hit en la búsqueda.
const snippetLen = 240

// newSearchCmd crea `nem search "<query>"`: búsqueda full-text (FTS5/BM25).
func newSearchCmd() *cobra.Command {
	var (
		top    int
		format string
		role   string
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search across all chats (full-text, BM25 ranking)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSearch(cmd, args[0], top, format, role)
		},
	}
	cmd.Flags().IntVar(&top, "top", 10, "number of results")
	cmd.Flags().StringVar(&format, "format", output.FormatMarkdown, "llm | json | markdown")
	cmd.Flags().StringVar(&role, "role", "", "roles to include, comma-separated (default: conversation + reasoning; 'all' = every role, incl. tool)")
	return cmd
}

func runSearch(cmd *cobra.Command, query string, top int, format, role string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	roles, err := resolveRoles(role)
	if err != nil {
		return err
	}
	allowed, _, err := resolveScope(cmd, store)
	if err != nil {
		return err
	}
	hits, err := store.SearchMessages(ftsQuery(query), top, roles, allowed)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(hits) == 0 {
		fmt.Fprintf(out, "no results for %q\n", query)
		return nil
	}

	switch format {
	case output.FormatJSON:
		return renderSearchJSON(cmd, query, hits)
	default:
		// llm y markdown comparten un layout compacto y escaneables.
		fmt.Fprintf(out, "%q — %d results\n\n", query, len(hits))
		for i, h := range hits {
			title := h.ChatTitle
			if title == "" {
				title = "(untitled)"
			}
			fmt.Fprintf(out, "%d. [%s · %s]  msg:%s\n", i+1, h.ChatSource, title, h.ID)
			fmt.Fprintf(out, "   %s: %s\n\n", h.Role, snippet(h.Content))
		}
		return nil
	}
}

func renderSearchJSON(cmd *cobra.Command, query string, hits []db.SearchHit) error {
	type jsonHit struct {
		MsgID   string  `json:"msg_id"`
		ChatID  string  `json:"chat_id"`
		Source  string  `json:"source"`
		Title   string  `json:"title"`
		Role    string  `json:"role"`
		Content string  `json:"content"`
		Score   float64 `json:"score"`
	}
	out := make([]jsonHit, 0, len(hits))
	for _, h := range hits {
		out = append(out, jsonHit{
			MsgID: h.ID, ChatID: h.ChatID, Source: h.ChatSource,
			Title: h.ChatTitle, Role: h.Role, Content: h.Content, Score: h.Score,
		})
	}
	b, err := json.MarshalIndent(map[string]any{"query": query, "hits": out}, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to render json: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}

// validRoles son los roles que nem persiste. "all" es un atajo especial.
var validRoles = map[string]bool{"user": true, "assistant": true, "reasoning": true, "tool": true}

// resolveRoles traduce el flag --role a la lista de roles para filtrar.
// Vacío = conversación + reasoning (excluye el ruido de tool). "all" = nil
// (todos los roles, incluido tool). Cualquier otro = lista coma-separada
// validada. Devuelve error si algún rol es desconocido.
func resolveRoles(flag string) ([]string, error) {
	flag = strings.TrimSpace(flag)
	switch flag {
	case "":
		return []string{"user", "assistant", "reasoning"}, nil
	case "all":
		return nil, nil
	default:
		parts := strings.Split(flag, ",")
		roles := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if !validRoles[p] {
				return nil, fmt.Errorf("unknown role %q (valid: user, assistant, reasoning, tool, all)", p)
			}
			roles = append(roles, p)
		}
		if len(roles) == 0 {
			return nil, fmt.Errorf("no valid roles in %q", flag)
		}
		return roles, nil
	}
}

// ftsQuery convierte un query libre en una query FTS5 segura: cada token se
// cita como literal y se combinan con AND implícito. Evita errores de sintaxis
// por puntuación (-, :, etc.) en la entrada del usuario.
func ftsQuery(raw string) string {
	fields := strings.Fields(raw)
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ReplaceAll(f, `"`, "")
		if f != "" {
			quoted = append(quoted, `"`+f+`"`)
		}
	}
	return strings.Join(quoted, " ")
}

// snippet acorta el contenido a una sola línea para el listado de búsqueda.
func snippet(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > snippetLen {
		return string(r[:snippetLen]) + "…"
	}
	return s
}
