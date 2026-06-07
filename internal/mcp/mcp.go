// Package mcp expone nem como servidor MCP (stdio) para que un agente lo use
// como herramientas tipadas, no solo por CLI. Es la integración premium: el
// agente llama nem_outline/search/read/timeline para navegar su memoria.
package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/index"
	"github.com/Dieg0Code/nem/internal/output"
	"github.com/Dieg0Code/nem/internal/retrieve"
	"github.com/Dieg0Code/nem/internal/scope"
	"github.com/Dieg0Code/nem/internal/session"
	"github.com/Dieg0Code/nem/internal/summarize"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// none es el tipo de salida para tools que devuelven solo texto.
type none struct{}

// Serve corre el servidor MCP de nem sobre stdio hasta que se cierre la conexión.
func Serve(ctx context.Context, store db.Store, version string) error {
	return newServer(store, version).Run(ctx, &mcp.StdioTransport{})
}

// newServer arma el servidor MCP con todas las tools (separado de Serve para
// poder testearlo con transportes in-memory).
func newServer(store db.Store, version string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "nem", Version: version}, nil)
	h := &handlers{store: store}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_outline",
		Description: "Show the index tree (table of contents) of the agent's memory. Start at a nodeID (e.g. project:foo, chat:id) or omit for the project roots. Read this first, then drill in.",
	}, h.outline)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_search",
		Description: "Hybrid search over memory (BM25 on messages + index nodes, RRF + recency). Returns candidates; you rerank by reading them.",
	}, h.search)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_read",
		Description: "Read content: a commit (HEAD, hash, or commit:hash) or a chat node (chat:id). Use to drill into what outline/search surfaced.",
	}, h.read)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_timeline",
		Description: "Show how a project's or chat's decisions (commits) evolved over time, oldest to newest (last = current).",
	}, h.timeline)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_status",
		Description: "Show the active detected session and its message/commit counts.",
	}, h.status)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_index",
		Description: "Rebuild the navigable index tree from chats and commits. Run after new sessions are ingested.",
	}, h.index)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "nem_commit",
		Description: "Persist context: snapshot the last N messages of a chat as an immutable commit with a message you write. Closes the memory write-loop.",
	}, h.commit)

	return server
}

type handlers struct {
	store db.Store
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

// allowed devuelve los chatIDs visibles según NEM_SCOPE (vacío = todos).
func (h *handlers) allowed() ([]string, bool, error) {
	name := os.Getenv("NEM_SCOPE")
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
	chats, err := h.store.ListChats()
	if err != nil {
		return nil, false, err
	}
	refs := make([]scope.ChatRef, len(chats))
	for i, c := range chats {
		refs[i] = scope.ChatRef{ID: c.ID, Title: c.Title, Source: c.Source}
	}
	ids, err := r.AllowedChatIDs(refs)
	return ids, true, err
}

// --- outline ---

type outlineIn struct {
	NodeID string `json:"node_id,omitempty" jsonschema:"start node (project:foo, chat:id); empty = project roots"`
	Depth  int    `json:"depth,omitempty" jsonschema:"levels to expand (default 2)"`
}

func (h *handlers) outline(ctx context.Context, _ *mcp.CallToolRequest, in outlineIn) (*mcp.CallToolResult, none, error) {
	depth := in.Depth
	if depth <= 0 {
		depth = 2
	}
	allowed, scoped, err := h.allowed()
	if err != nil {
		return nil, none{}, err
	}
	var roots []db.Node
	if in.NodeID == "" {
		roots, err = h.store.RootNodes()
	} else {
		n, e := h.store.GetNode(in.NodeID)
		if e != nil {
			return nil, none{}, e
		}
		if n == nil {
			return nil, none{}, fmt.Errorf("node %q not found (run nem_index?)", in.NodeID)
		}
		roots = []db.Node{*n}
	}
	if err != nil {
		return nil, none{}, err
	}
	var b strings.Builder
	if len(roots) == 0 {
		b.WriteString("empty index — call nem_index first\n")
	}
	for _, r := range roots {
		h.walk(&b, r, 0, depth, allowed, scoped)
	}
	return textResult(b.String()), none{}, nil
}

func (h *handlers) walk(b *strings.Builder, n db.Node, level, depth int, allowed []string, scoped bool) {
	if scoped && n.ChatID != "" && !slices.Contains(allowed, n.ChatID) {
		return
	}
	indent := strings.Repeat("  ", level)
	fmt.Fprintf(b, "%s- [%s] %s", indent, n.Kind, n.Title)
	if s := strings.TrimSpace(n.Summary); s != "" && n.Kind != "project" {
		fmt.Fprintf(b, " — %s", s)
	}
	fmt.Fprintf(b, "  (%s)\n", n.ID)
	if level+1 >= depth {
		return
	}
	children, err := h.store.ChildNodes(n.ID)
	if err != nil {
		return
	}
	for _, c := range children {
		h.walk(b, c, level+1, depth, allowed, scoped)
	}
}

// --- search ---

type searchIn struct {
	Query string `json:"query" jsonschema:"the search query"`
	Top   int    `json:"top,omitempty" jsonschema:"max results (default 10)"`
	Mode  string `json:"mode,omitempty" jsonschema:"hybrid (default) or keyword"`
	Role  string `json:"role,omitempty" jsonschema:"message roles to include; 'all' includes tool output"`
}

func (h *handlers) search(ctx context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, none, error) {
	top := in.Top
	if top <= 0 {
		top = 10
	}
	allowed, _, err := h.allowed()
	if err != nil {
		return nil, none{}, err
	}
	roles := rolesFor(in.Role)
	fts := ftsQuery(in.Query)
	fetch := top * 2

	var channels []retrieve.Channel
	msgHits, err := h.store.SearchMessages(fts, fetch, roles, allowed)
	if err != nil {
		return nil, none{}, err
	}
	items := make([]retrieve.Item, 0, len(msgHits))
	for _, m := range msgHits {
		items = append(items, retrieve.Item{Kind: "message", ID: m.ID, ChatID: m.ChatID, Title: m.ChatTitle, Source: m.ChatSource, Role: m.Role, Content: m.Content, Timestamp: m.Timestamp})
	}
	channels = append(channels, retrieve.Channel{Name: "messages", Items: items})
	if in.Mode != "keyword" {
		nodeHits, err := h.store.SearchNodes(fts, fetch, allowed)
		if err != nil {
			return nil, none{}, err
		}
		nitems := make([]retrieve.Item, 0, len(nodeHits))
		for _, nh := range nodeHits {
			nitems = append(nitems, retrieve.Item{Kind: "node", ID: nh.ID, ChatID: nh.ChatID, Title: nh.Title, NodeKind: nh.Kind, Content: nh.Summary, Timestamp: nh.CreatedAt})
		}
		channels = append(channels, retrieve.Channel{Name: "nodes", Items: nitems})

		if vc, err := retrieve.VectorChannel(h.store, in.Query, fetch, allowed); err != nil {
			return nil, none{}, err
		} else if vc != nil {
			channels = append(channels, *vc)
		}
	}
	results := retrieve.Fuse(channels, top)

	var b strings.Builder
	if len(results) == 0 {
		fmt.Fprintf(&b, "no results for %q\n", in.Query)
		return textResult(b.String()), none{}, nil
	}
	for i, r := range results {
		title := r.Title
		if title == "" {
			title = "(untitled)"
		}
		if r.Kind == "node" {
			fmt.Fprintf(&b, "%d. [index:%s · %s] %s\n   %s\n", i+1, r.NodeKind, title, r.ID, oneLine(r.Content, 240))
		} else {
			fmt.Fprintf(&b, "%d. [%s · %s] msg:%s\n   %s: %s\n", i+1, r.Source, title, r.ID, r.Role, oneLine(r.Content, 240))
		}
	}
	return textResult(b.String()), none{}, nil
}

// --- read ---

type readIn struct {
	Ref    string `json:"ref" jsonschema:"HEAD, a commit hash, commit:hash, or chat:id"`
	Format string `json:"format,omitempty" jsonschema:"llm (default), json, or markdown"`
}

func (h *handlers) read(ctx context.Context, _ *mcp.CallToolRequest, in readIn) (*mcp.CallToolResult, none, error) {
	format := in.Format
	if format == "" {
		format = output.FormatLLM
	}
	allowed, scoped, err := h.allowed()
	if err != nil {
		return nil, none{}, err
	}

	if strings.HasPrefix(in.Ref, "chat:") {
		chatID := strings.TrimPrefix(in.Ref, "chat:")
		if scoped && !slices.Contains(allowed, chatID) {
			return nil, none{}, fmt.Errorf("chat %q out of scope", chatID)
		}
		chat, err := h.store.GetChat(chatID)
		if err != nil || chat == nil {
			return nil, none{}, fmt.Errorf("chat %q not found", chatID)
		}
		msgs, err := h.store.LastMessages(chatID, 40, []string{"user", "assistant", "reasoning"})
		if err != nil {
			return nil, none{}, err
		}
		snap := make([]output.SnapMessage, 0, len(msgs))
		for _, m := range msgs {
			snap = append(snap, output.SnapMessage{Role: m.Role, Content: m.Content, Timestamp: m.Timestamp, Seq: m.Seq})
		}
		s, err := output.Render(output.Doc{Title: chat.Title, Source: chat.Source, Date: time.Unix(chat.CreatedAt, 0), Messages: snap}, format)
		if err != nil {
			return nil, none{}, err
		}
		return textResult(s), none{}, nil
	}

	ref := strings.TrimPrefix(in.Ref, "commit:")
	var commit *db.Commit
	if strings.EqualFold(ref, "HEAD") {
		chatID := detectChatID()
		if chatID == "" {
			return nil, none{}, fmt.Errorf("no active session for HEAD")
		}
		commit, err = h.store.HeadCommit(chatID)
	} else {
		commit, err = h.store.GetCommit(ref)
	}
	if err != nil {
		return nil, none{}, err
	}
	if commit == nil {
		return nil, none{}, fmt.Errorf("commit %q not found", in.Ref)
	}
	if scoped && !slices.Contains(allowed, commit.ChatID) {
		return nil, none{}, fmt.Errorf("commit %q out of scope", in.Ref)
	}
	snap, err := output.ParseSnapshot(commit.Snapshot)
	if err != nil {
		return nil, none{}, err
	}
	doc := output.Doc{Date: time.Unix(commit.CreatedAt, 0), Messages: snap, Commit: commit}
	if chat, _ := h.store.GetChat(commit.ChatID); chat != nil {
		doc.Title, doc.Source = chat.Title, chat.Source
	}
	s, err := output.Render(doc, format)
	if err != nil {
		return nil, none{}, err
	}
	return textResult(s), none{}, nil
}

// --- timeline ---

type timelineIn struct {
	Target string `json:"target" jsonschema:"a project name (chat title) or a chat id"`
}

func (h *handlers) timeline(ctx context.Context, _ *mcp.CallToolRequest, in timelineIn) (*mcp.CallToolResult, none, error) {
	chats, err := h.store.ListChats()
	if err != nil {
		return nil, none{}, err
	}
	var chatIDs []string
	for _, c := range chats {
		if c.Title == in.Target {
			chatIDs = append(chatIDs, c.ID)
		}
	}
	if len(chatIDs) == 0 {
		chatIDs = []string{in.Target}
	}
	nodes, err := h.store.CommitNodes(chatIDs)
	if err != nil {
		return nil, none{}, err
	}
	var b strings.Builder
	if len(nodes) == 0 {
		fmt.Fprintf(&b, "no commits for %q\n", in.Target)
	}
	for i, n := range nodes {
		marker := ""
		if i == len(nodes)-1 {
			marker = "  ← current"
		}
		fmt.Fprintf(&b, "%s  %s  %s%s\n", time.Unix(n.CreatedAt, 0).Format("2006-01-02 15:04"), short(n.CommitHash), n.Title, marker)
	}
	return textResult(b.String()), none{}, nil
}

// --- status ---

func (h *handlers) status(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, none, error) {
	chatID := detectChatID()
	if chatID == "" {
		return textResult("no active session detected"), none{}, nil
	}
	var b strings.Builder
	chat, _ := h.store.GetChat(chatID)
	count, _ := h.store.CountMessages(chatID)
	head, _ := h.store.HeadCommit(chatID)
	title := chatID
	source := ""
	if chat != nil {
		title, source = chat.Title, chat.Source
	}
	fmt.Fprintf(&b, "active session: %s · %s\nchat: %s\nmessages: %d\n", source, title, chatID, count)
	if head != nil {
		fmt.Fprintf(&b, "HEAD: %s %q\n", short(head.Hash), head.Message)
	}
	return textResult(b.String()), none{}, nil
}

// --- index ---

func (h *handlers) index(ctx context.Context, _ *mcp.CallToolRequest, _ none) (*mcp.CallToolResult, none, error) {
	var opts []index.Option
	if sum, e := summarize.FromConfig(); e == nil && sum != nil {
		opts = append(opts, index.WithSummarizer(sum))
	}
	b, err := index.New(h.store, opts...)
	if err != nil {
		return nil, none{}, err
	}
	rep, err := b.Build()
	if err != nil {
		return nil, none{}, err
	}
	return textResult(fmt.Sprintf("indexed: %d projects, %d chats, %d commits (%d nodes)", rep.Projects, rep.Chats, rep.Commits, rep.Nodes)), none{}, nil
}

// --- commit ---

type commitIn struct {
	Message string `json:"message" jsonschema:"the commit message you write (the decision, imperative)"`
	ChatID  string `json:"chat_id,omitempty" jsonschema:"chat to commit (default: detected session)"`
	LastN   int    `json:"last_n,omitempty" jsonschema:"how many recent conversation messages to snapshot (default 10)"`
}

func (h *handlers) commit(ctx context.Context, _ *mcp.CallToolRequest, in commitIn) (*mcp.CallToolResult, none, error) {
	if strings.TrimSpace(in.Message) == "" {
		return nil, none{}, fmt.Errorf("message is required")
	}
	chatID := in.ChatID
	if chatID == "" {
		chatID = detectChatID()
	}
	if chatID == "" {
		return nil, none{}, fmt.Errorf("no chat: pass chat_id or open a session")
	}
	n := in.LastN
	if n <= 0 {
		n = 10
	}
	msgs, err := h.store.LastMessages(chatID, n, []string{"user", "assistant", "reasoning"})
	if err != nil {
		return nil, none{}, err
	}
	if len(msgs) == 0 {
		return nil, none{}, fmt.Errorf("no messages to commit in %s", chatID)
	}
	snapshot, err := output.BuildSnapshot(msgs)
	if err != nil {
		return nil, none{}, err
	}
	hsh := sha256.New()
	fmt.Fprintf(hsh, "%s\n%s\n%d\n%s", chatID, in.Message, time.Now().UnixNano(), snapshot)
	commit := &db.Commit{
		Hash: hex.EncodeToString(hsh.Sum(nil)), ChatID: chatID, Branch: "main",
		Message: in.Message, MsgFrom: msgs[0].ID, MsgTo: msgs[len(msgs)-1].ID,
		Snapshot: snapshot, CreatedAt: time.Now().Unix(),
	}
	if err := h.store.CreateCommit(commit); err != nil {
		return nil, none{}, err
	}
	return textResult(fmt.Sprintf("commit %s: %q (%d messages)", short(commit.Hash), in.Message, len(msgs))), none{}, nil
}

// --- helpers ---

func detectChatID() string {
	d, err := session.NewDetector()
	if err != nil {
		return ""
	}
	s, err := d.Detect()
	if err != nil || s == nil {
		return ""
	}
	return s.ChatID
}

func rolesFor(flag string) []string {
	flag = strings.TrimSpace(flag)
	switch flag {
	case "":
		return []string{"user", "assistant", "reasoning"}
	case "all":
		return nil
	default:
		var roles []string
		for _, p := range strings.Split(flag, ",") {
			if p = strings.TrimSpace(p); p != "" {
				roles = append(roles, p)
			}
		}
		return roles
	}
}

func ftsQuery(raw string) string {
	var quoted []string
	for _, f := range strings.Fields(raw) {
		f = strings.ReplaceAll(f, `"`, "")
		if f != "" {
			quoted = append(quoted, `"`+f+`"`)
		}
	}
	return strings.Join(quoted, " ")
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " ")), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func short(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}
