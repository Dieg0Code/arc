package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func mustStore(t *testing.T) db.Store {
	t.Helper()
	s, err := db.New(db.WithPath(filepath.Join(t.TempDir(), "nem.db")))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func callText(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s: %v", name, err)
	}
	if len(res.Content) == 0 {
		t.Fatalf("CallTool %s: empty content", name)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("CallTool %s: content[0] not text", name)
	}
	return tc.Text
}

func TestMCP_Tools(t *testing.T) {
	store := mustStore(t)
	if err := store.UpsertChat(&db.Chat{ID: "c1", Title: "demo", Source: "codex", CreatedAt: 1000}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertNodes([]db.Node{
		{ID: "project:demo", Kind: "project", Title: "demo", Summary: "demo project", CreatedAt: 1000},
		{ID: "chat:c1", ParentID: "project:demo", Kind: "chat", ChatID: "c1", Title: "demo", Summary: "decay logic for inactive players", CreatedAt: 1000},
	}); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	clientT, serverT := mcp.NewInMemoryTransports()
	srv := newServer(store, "test")
	go func() { _ = srv.Run(ctx, serverT) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	cs, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer cs.Close()

	// outline debe listar el proyecto.
	if out := callText(t, cs, "nem_outline", map[string]any{"depth": 1}); !strings.Contains(out, "project:demo") {
		t.Errorf("outline missing project: %q", out)
	}
	// search por el summary del nodo chat.
	if out := callText(t, cs, "nem_search", map[string]any{"query": "decay", "top": 5}); !strings.Contains(out, "chat:c1") {
		t.Errorf("search missing chat node: %q", out)
	}
	// index reconstruye el árbol.
	if out := callText(t, cs, "nem_index", map[string]any{}); !strings.Contains(out, "indexed:") {
		t.Errorf("index unexpected: %q", out)
	}
}
