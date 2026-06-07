// Package index construye el árbol de índice de nem (estilo PageIndex): una
// tabla de contenidos jerárquica (project → chat → commit) que el agente navega
// y razona, sin embeddings. Los commits aportan su resumen gratis (el mensaje
// escrito por el agente); los chats crudos se resumen heurísticamente (o con un
// summarizer pluggable inyectado en fases posteriores).
package index

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/embed"
	"github.com/Dieg0Code/nem/internal/summarize"
)

// summaryTextBudget acota cuánto texto de la conversación se le pasa al LLM.
// Más chico = generación más rápida (importa en CPU, donde los prompts largos
// hacen lento/flaky al modelo local).
const summaryTextBudget = 1500

// maxSummaryChars acota el resumen de un nodo (presupuesto de tokens del outline).
const maxSummaryChars = 280

// SummaryFunc produce el resumen de un chat a partir de sus primeros mensajes.
// Se inyecta para permitir backends (heurístico por defecto; Ollama/API luego).
type SummaryFunc func(chat db.Chat, firstMsgs []db.Message) string

// Report resume una corrida de Build.
type Report struct {
	Projects int
	Chats    int
	Commits  int
	Nodes    int
	Embedded int
}

// Builder construye/refresca el árbol de índice.
type Builder interface {
	// Build reconstruye el árbol completo (borra y regenera). Idempotente.
	Build() (*Report, error)
}

// ProgressFunc recibe el avance de Build (stage = "summarize" | "embed").
type ProgressFunc func(stage string, done, total int)

type config struct {
	summary  SummaryFunc
	embedder embed.Embedder
	progress ProgressFunc
}

// Option configura al Builder.
type Option func(*config) error

// WithSummaryFunc reemplaza el resumidor de chats (default: heurístico).
func WithSummaryFunc(fn SummaryFunc) Option {
	return func(c *config) error {
		if fn == nil {
			return errors.New("summary func cannot be nil")
		}
		c.summary = fn
		return nil
	}
}

// WithSummarizer usa un Summarizer con LLM (Ollama/API) para los resúmenes de
// chat, con fallback al heurístico si el backend falla o devuelve vacío.
func WithSummarizer(s summarize.Summarizer) Option {
	return func(c *config) error {
		if s == nil {
			return errors.New("summarizer cannot be nil")
		}
		// Tolera fallos transitorios (p.ej. el primer request mientras el modelo
		// carga en frío) y solo se rinde —pasando todo a heurístico— tras varios
		// fallos seguidos (backend realmente caído), evitando colgar con N
		// timeouts. Build() es secuencial, así que el contador no necesita sync.
		const giveUpAfter = 3
		fails := 0
		c.summary = func(chat db.Chat, msgs []db.Message) string {
			if fails >= giveUpAfter {
				return HeuristicSummary(chat, msgs)
			}
			out, err := s.Summarize(context.Background(), chat.Title, joinForSummary(msgs))
			if err != nil || strings.TrimSpace(out) == "" {
				fails++
				return HeuristicSummary(chat, msgs)
			}
			fails = 0 // reset al primer éxito
			return truncate(out, maxSummaryChars)
		}
		return nil
	}
}

// WithProgress recibe callbacks de avance (para mostrar "N/total" en el CLI).
func WithProgress(fn ProgressFunc) Option {
	return func(c *config) error {
		c.progress = fn
		return nil
	}
}

// WithEmbedder activa la capa de embeddings: tras construir el árbol, embebe los
// resúmenes de los nodos y los guarda. Si el backend falla, el índice igual se
// construye (los embeddings simplemente no se generan).
func WithEmbedder(e embed.Embedder) Option {
	return func(c *config) error {
		if e == nil {
			return errors.New("embedder cannot be nil")
		}
		c.embedder = e
		return nil
	}
}

// joinForSummary arma el texto (acotado) que se le pasa al summarizer.
func joinForSummary(msgs []db.Message) string {
	var b strings.Builder
	for _, m := range msgs {
		if b.Len() > summaryTextBudget {
			break
		}
		fmt.Fprintf(&b, "%s: %s\n", m.Role, oneLine(m.Content))
	}
	return b.String()
}

type builder struct {
	store    db.Store
	summary  SummaryFunc
	embedder embed.Embedder
	progress ProgressFunc
}

// New crea un Builder sobre el store dado.
func New(store db.Store, options ...Option) (Builder, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	cfg := &config{summary: HeuristicSummary}
	for _, option := range options {
		if err := option(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply index option: %w", err)
		}
	}
	return &builder{store: store, summary: cfg.summary, embedder: cfg.embedder, progress: cfg.progress}, nil
}

// Build hace un rebuild completo: project → chat → commit.
func (b *builder) Build() (*Report, error) {
	chats, err := b.store.ListChats()
	if err != nil {
		return nil, err
	}
	if err := b.store.ClearNodes(); err != nil {
		return nil, err
	}

	rep := &Report{}
	var nodes []db.Node
	seenProject := map[string]bool{}

	for i, chat := range chats {
		if b.progress != nil {
			b.progress("summarize", i+1, len(chats))
		}
		proj := projectKey(chat.Title)
		projID := "project:" + proj
		if !seenProject[projID] {
			seenProject[projID] = true
			nodes = append(nodes, db.Node{
				ID:        projID,
				ParentID:  "",
				Kind:      "project",
				Title:     proj,
				Summary:   "Project: " + proj,
				CreatedAt: chat.CreatedAt,
			})
			rep.Projects++
		}

		// Chat node: resumen desde los primeros mensajes de conversación.
		first, err := b.store.MessagesBySeqRange(chat.ID, 1, 8, []string{"user", "assistant"})
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, db.Node{
			ID:         "chat:" + chat.ID,
			ParentID:   projID,
			Kind:       "chat",
			ChatID:     chat.ID,
			Title:      chatTitle(chat),
			Summary:    truncate(b.summary(chat, first), maxSummaryChars),
			CreatedAt:  chat.CreatedAt,
			MsgFromSeq: 1,
		})
		rep.Chats++

		// Commit nodes: el mensaje del commit ES el resumen (gratis).
		commits, err := b.store.ListCommits(chat.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range commits {
			fromSeq, toSeq := b.commitRange(chat.ID, c)
			nodes = append(nodes, db.Node{
				ID:         "commit:" + c.Hash,
				ParentID:   "chat:" + chat.ID,
				Kind:       "commit",
				ChatID:     chat.ID,
				Title:      firstLine(c.Message, 70),
				Summary:    truncate(c.Message, maxSummaryChars),
				CommitHash: c.Hash,
				MsgFromSeq: fromSeq,
				MsgToSeq:   toSeq,
				CreatedAt:  c.CreatedAt,
			})
			rep.Commits++
		}
	}

	if _, err := b.store.UpsertNodes(nodes); err != nil {
		return nil, err
	}
	rep.Nodes = len(nodes)

	if b.embedder != nil {
		if b.progress != nil {
			b.progress("embed", len(nodes), len(nodes))
		}
		rep.Embedded = b.embedNodes(nodes)
	}
	return rep, nil
}

// embedNodes embebe los resúmenes de los nodos y los guarda. Devuelve cuántos se
// guardaron (0 si el backend falla; el árbol ya quedó construido igual).
func (b *builder) embedNodes(nodes []db.Node) int {
	texts := make([]string, len(nodes))
	for i, n := range nodes {
		texts[i] = strings.TrimSpace(n.Title + "\n" + n.Summary)
	}
	vecs, err := b.embedder.Embed(context.Background(), texts)
	if err != nil || len(vecs) != len(nodes) {
		return 0
	}
	embs := make([]db.Embedding, 0, len(nodes))
	for i, v := range vecs {
		if len(v) == 0 {
			continue
		}
		embs = append(embs, db.Embedding{NodeID: nodes[i].ID, Dim: len(v), Vec: embed.Encode(v)})
	}
	_ = b.store.ClearEmbeddings()
	n, _ := b.store.UpsertEmbeddings(embs)
	return int(n)
}

// commitRange resuelve el rango de Seq que cubre un commit (sus MsgFrom/MsgTo
// son ids de mensaje). Si no se resuelven, devuelve 0,0.
func (b *builder) commitRange(chatID string, c db.Commit) (int64, int64) {
	var from, to int64
	if m, err := b.store.MessageByID(chatID, c.MsgFrom); err == nil && m != nil {
		from = m.Seq
	}
	if m, err := b.store.MessageByID(chatID, c.MsgTo); err == nil && m != nil {
		to = m.Seq
	}
	return from, to
}

// HeuristicSummary arma un resumen barato (sin LLM): el primer mensaje de usuario
// (la tarea/tema), con fallback al primer mensaje disponible.
func HeuristicSummary(chat db.Chat, firstMsgs []db.Message) string {
	for _, m := range firstMsgs {
		if m.Role == "user" && strings.TrimSpace(m.Content) != "" {
			return oneLine(m.Content)
		}
	}
	for _, m := range firstMsgs {
		if strings.TrimSpace(m.Content) != "" {
			return oneLine(m.Content)
		}
	}
	if chat.Title != "" {
		return chat.Title
	}
	return "(empty chat)"
}

// projectKey normaliza el título del chat a una clave de proyecto.
func projectKey(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return "untitled"
	}
	return t
}

func chatTitle(chat db.Chat) string {
	if strings.TrimSpace(chat.Title) != "" {
		return chat.Title
	}
	return chat.ID
}

func oneLine(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")
	return strings.Join(strings.Fields(s), " ")
}

func firstLine(s string, max int) string {
	s = oneLine(s)
	return truncate(s, max)
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
