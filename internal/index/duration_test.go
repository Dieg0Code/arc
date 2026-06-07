package index

import (
	"path/filepath"
	"testing"

	"github.com/Dieg0Code/nem/internal/db"
)

func TestBuildPopulatesDurations(t *testing.T) {
	const base = int64(1_700_000_000)
	store, err := db.New(db.WithPath(filepath.Join(t.TempDir(), "nem.db")))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.UpsertChat(&db.Chat{ID: "c1", Title: "projX", Source: "manual", CreatedAt: base}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	// user@base, assistant@base+600 → hueco de trabajo de 10 min (activo=600s).
	_, err = store.InsertMessages([]db.Message{
		{ID: "m1", ChatID: "c1", Role: "user", Content: "hagamos la feature X", Timestamp: base, Seq: 1},
		{ID: "m2", ChatID: "c1", Role: "assistant", Content: "listo, hecho", Timestamp: base + 600, Seq: 2},
	})
	if err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	b, err := New(store)
	if err != nil {
		t.Fatalf("index.New: %v", err)
	}
	if _, err := b.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}

	chat, err := store.GetNode("chat:c1")
	if err != nil || chat == nil {
		t.Fatalf("GetNode chat: %v", err)
	}
	if chat.ActiveSecs != 600 {
		t.Errorf("chat ActiveSecs = %d, want 600", chat.ActiveSecs)
	}
	if chat.Sessions != 1 {
		t.Errorf("chat Sessions = %d, want 1", chat.Sessions)
	}
	if chat.LastActive != base+600 {
		t.Errorf("chat LastActive = %d, want %d", chat.LastActive, base+600)
	}

	proj, err := store.GetNode("project:projX")
	if err != nil || proj == nil {
		t.Fatalf("GetNode project: %v", err)
	}
	if proj.ActiveSecs != 600 {
		t.Errorf("project ActiveSecs = %d, want 600 (aggregated from its one chat)", proj.ActiveSecs)
	}
}
