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
	// Build refresca el árbol (project → chat → commit). Es INCREMENTAL: reusa
	// los resúmenes y embeddings que ya existen y solo computa lo nuevo/cambiado
	// (chats sin resumen, commits nuevos), salvo que se use WithForce. Idempotente.
	Build() (*Report, error)
}

// ProgressFunc recibe el avance de Build (stage = "summarize" | "embed").
type ProgressFunc func(stage string, done, total int)

type config struct {
	summary  SummaryFunc
	embedder embed.Embedder
	progress ProgressFunc
	force    bool
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

// WithForce fuerza recomputar TODOS los resúmenes y embeddings, ignorando los
// que ya existen. Sin esto, Build es incremental: reusa el trabajo previo y solo
// calcula lo nuevo/cambiado (chats sin resumen, commits nuevos).
func WithForce(force bool) Option {
	return func(c *config) error {
		c.force = force
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
	force    bool
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
	return &builder{store: store, summary: cfg.summary, embedder: cfg.embedder, progress: cfg.progress, force: cfg.force}, nil
}

// Build refresca el árbol (project → chat → commit) de forma incremental.
func (b *builder) Build() (*Report, error) {
	chats, err := b.store.ListChats()
	if err != nil {
		return nil, err
	}

	// Resúmenes Pinned (escritos por el agente/humano): nunca se auto-pisan, ni
	// siquiera con --force. Es la capa mutable sobre los commits inmutables.
	pinned := map[string]string{}
	if pn, e := b.store.PinnedNodes(); e == nil {
		for _, n := range pn {
			pinned[n.ID] = n.Summary
		}
	}

	// Snapshot de los resúmenes ya existentes ANTES de limpiar: así reusamos el
	// trabajo hecho (chats) y detectamos si un resumen de proyecto cambió, en vez
	// de re-llamar al LLM o re-embeber por gusto.
	prevSummary := map[string]string{}
	if !b.force {
		for _, chat := range chats {
			if n, e := b.store.GetNode("chat:" + chat.ID); e == nil && n != nil && strings.TrimSpace(n.Summary) != "" {
				prevSummary[chat.ID] = n.Summary
			}
		}
	}
	prevProj := map[string]string{}
	for _, chat := range chats {
		pid := "project:" + projectKey(chat.Title)
		if _, ok := prevProj[pid]; !ok {
			if n, e := b.store.GetNode(pid); e == nil && n != nil {
				prevProj[pid] = n.Summary
			}
		}
	}
	// Cuántos chats necesitan resumen de verdad (para el progreso "N/total").
	todo := 0
	for _, chat := range chats {
		id := "chat:" + chat.ID
		if _, p := pinned[id]; p {
			continue
		}
		if _, ok := prevSummary[chat.ID]; !ok {
			todo++
		}
	}

	if err := b.store.ClearNodes(); err != nil {
		return nil, err
	}

	rep := &Report{}
	var nodes []db.Node
	projPos := map[string]int{}           // projID → posición en nodes (para sintetizar su resumen al final)
	projChatSums := map[string][]string{} // projID → resúmenes de sus chats
	fresh := map[string]bool{}            // node IDs cuyo contenido es nuevo/cambiado (para re-embeber)
	done := 0

	for _, chat := range chats {
		proj := projectKey(chat.Title)
		projID := "project:" + proj
		if _, seen := projPos[projID]; !seen {
			nodes = append(nodes, db.Node{
				ID:        projID,
				ParentID:  "",
				Kind:      "project",
				Title:     proj,
				Summary:   "Project: " + proj, // placeholder; se sintetiza al final
				CreatedAt: chat.CreatedAt,
			})
			projPos[projID] = len(nodes) - 1
			rep.Projects++
		}

		// Chat node: pinned > resumen previo (reuso) > generar.
		chatNodeID := "chat:" + chat.ID
		summary, isPinned := pinned[chatNodeID]
		if !isPinned {
			var ok bool
			summary, ok = prevSummary[chat.ID]
			if !ok {
				// Solo acá tocamos el summarizer (y leemos los primeros mensajes).
				first, err := b.store.MessagesBySeqRange(chat.ID, 1, 10, []string{"user", "assistant"})
				if err != nil {
					return nil, err
				}
				done++
				if b.progress != nil {
					b.progress("summarize", done, todo)
				}
				summary = truncate(b.summary(chat, first), maxSummaryChars)
				fresh[chatNodeID] = true // cambió → re-embeber
			}
		}
		nodes = append(nodes, db.Node{
			ID:         chatNodeID,
			ParentID:   projID,
			Kind:       "chat",
			ChatID:     chat.ID,
			Title:      chatTitle(chat),
			Summary:    summary,
			CreatedAt:  chat.CreatedAt,
			MsgFromSeq: 1,
			Pinned:     isPinned,
		})
		projChatSums[projID] = append(projChatSums[projID], summary)
		rep.Chats++

		// Commit nodes: el mensaje del commit ES el resumen (gratis), salvo pinned.
		commits, err := b.store.ListCommits(chat.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range commits {
			fromSeq, toSeq := b.commitRange(chat.ID, c)
			commitNodeID := "commit:" + c.Hash
			cSummary, cPinned := pinned[commitNodeID]
			if !cPinned {
				cSummary = truncate(c.Message, maxSummaryChars)
			}
			nodes = append(nodes, db.Node{
				ID:         commitNodeID,
				ParentID:   chatNodeID,
				Kind:       "commit",
				ChatID:     chat.ID,
				Title:      firstLine(c.Message, 70),
				Summary:    cSummary,
				CommitHash: c.Hash,
				MsgFromSeq: fromSeq,
				MsgToSeq:   toSeq,
				CreatedAt:  c.CreatedAt,
				Pinned:     cPinned,
			})
			rep.Commits++
		}
	}

	// Resumen de proyecto: sintetizado de sus chats (mejor que "Project: X"),
	// salvo que esté pinned. Solo se marca fresh si cambió respecto al previo.
	for projID, pos := range projPos {
		if ps, ok := pinned[projID]; ok {
			nodes[pos].Summary = ps
			nodes[pos].Pinned = true
			continue
		}
		s := projectSummary(nodes[pos].Title, projChatSums[projID])
		nodes[pos].Summary = s
		if prevProj[projID] != s {
			fresh[projID] = true
		}
	}

	if _, err := b.store.UpsertNodes(nodes); err != nil {
		return nil, err
	}
	rep.Nodes = len(nodes)

	if b.embedder != nil {
		rep.Embedded = b.embedNodes(nodes, fresh)
	}
	return rep, nil
}

// projectSummary sintetiza el resumen de un proyecto a partir de los resúmenes de
// sus chats (mucho más útil que "Project: X" para navegar y para los embeddings).
func projectSummary(title string, chatSums []string) string {
	seen := map[string]bool{}
	var parts []string
	for _, s := range chatSums {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "Project: " + title
	}
	head := fmt.Sprintf("%d chats. ", len(chatSums))
	if len(chatSums) == 1 {
		head = "1 chat. "
	}
	return truncate(head+strings.Join(parts, " · "), maxSummaryChars)
}

// embedNodes embebe los resúmenes de los nodos de forma incremental: reusa los
// embeddings existentes y solo recomputa los nodos nuevos o cuyo resumen cambió
// (marcados en fresh). Devuelve cuántos quedaron guardados. Si el backend falla,
// conserva los embeddings previos (el árbol ya quedó construido igual).
func (b *builder) embedNodes(nodes []db.Node, fresh map[string]bool) int {
	prev, _ := b.store.AllEmbeddings()
	have := make(map[string]db.Embedding, len(prev))
	for _, e := range prev {
		have[e.NodeID] = e
	}

	// Qué nodos hay que embeber: los que no tienen embedding o cuyo texto cambió.
	var idx []int
	var texts []string
	for i, n := range nodes {
		if _, ok := have[n.ID]; !ok || fresh[n.ID] || b.force {
			idx = append(idx, i)
			texts = append(texts, strings.TrimSpace(n.Title+"\n"+n.Summary))
		}
	}

	newVecs := map[string][]float32{}
	if len(texts) > 0 {
		if b.progress != nil {
			b.progress("embed", len(texts), len(texts))
		}
		vecs, err := b.embedder.Embed(context.Background(), texts)
		if err != nil || len(vecs) != len(texts) {
			return len(prev) // backend caído: conservamos lo que había
		}
		for j, i := range idx {
			newVecs[nodes[i].ID] = vecs[j]
		}
	}

	// Set final: embedding nuevo si lo recomputamos, si no el previo (reuso).
	// Recorrer nodes (no prev) descarta de paso embeddings de nodos borrados.
	embs := make([]db.Embedding, 0, len(nodes))
	for _, n := range nodes {
		if v, ok := newVecs[n.ID]; ok {
			if len(v) == 0 {
				continue
			}
			embs = append(embs, db.Embedding{NodeID: n.ID, Dim: len(v), Vec: embed.Encode(v)})
		} else if e, ok := have[n.ID]; ok {
			embs = append(embs, e)
		}
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
