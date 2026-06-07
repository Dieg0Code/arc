package sync

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dieg0Code/nem/internal/db"
	"github.com/Dieg0Code/nem/internal/output"
)

// seedStore crea un store temporal con un chat, mensajes y un commit cuyo
// snapshot contiene un secreto.
func seedStore(t *testing.T) (db.Store, string) {
	t.Helper()
	store, err := db.New(db.WithPath(filepath.Join(t.TempDir(), "nem.db")))
	if err != nil {
		t.Fatalf("db.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.UpsertChat(&db.Chat{ID: "c1", Title: "proj", Source: "codex", CreatedAt: 1700000000}); err != nil {
		t.Fatal(err)
	}
	msgs := []db.Message{
		// Token armado por fragmentos: evita un literal con forma de secreto en
		// el fuente (que dispararía el secret-scanning de GitHub).
		{ID: "c1:1", ChatID: "c1", Role: "user", Content: "mi token es " + "hf_" + "abcdefghijklmnopqrstuvwxyz123456" + " ok", Seq: 1},
		{ID: "c1:2", ChatID: "c1", Role: "assistant", Content: "listo, lo guardo", Seq: 2},
	}
	if _, err := store.InsertMessages(msgs); err != nil {
		t.Fatal(err)
	}
	snap, _ := output.BuildSnapshot(msgs)
	commit := &db.Commit{
		Hash: "deadbeefcafe", ChatID: "c1", Branch: "main",
		Message: "guarda token", Snapshot: snap, CreatedAt: 1700000100,
		MsgFrom: "c1:1", MsgTo: "c1:2",
	}
	if err := store.CreateCommit(commit); err != nil {
		t.Fatal(err)
	}
	return store, "c1"
}

func TestSync_ExportRedactsSecrets(t *testing.T) {
	store, _ := seedStore(t)
	dir := t.TempDir()
	sy, err := NewSyncer(store, WithDir(dir))
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}
	s := sy.(*syncer)

	n, counts, err := s.export()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if n != 1 {
		t.Fatalf("exported %d commits, want 1", n)
	}
	if counts["huggingface-token"] != 1 {
		t.Errorf("expected 1 hf token redacted, counts=%v", counts)
	}

	// El archivo exportado NO debe contener el secreto en claro.
	data, err := os.ReadFile(filepath.Join(dir, "store", "chats", "deadbeefcafe.jsonl"))
	if err != nil {
		t.Fatalf("read export: %v", err)
	}
	if strings.Contains(string(data), "hf_abcdefghij") {
		t.Errorf("secret leaked into export file:\n%s", data)
	}
	if !strings.Contains(string(data), "REDACTED:huggingface-token") {
		t.Errorf("placeholder missing in export file:\n%s", data)
	}
}

func TestSync_ImportRoundTrip(t *testing.T) {
	src, _ := seedStore(t)
	dir := t.TempDir()
	srcSync := mustSyncer(t, src, dir)
	if _, _, err := srcSync.export(); err != nil {
		t.Fatalf("export: %v", err)
	}

	// Store destino vacío: importar desde el mismo dir.
	dst, err := db.New(db.WithPath(filepath.Join(t.TempDir(), "dst.db")))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	dstSync := mustSyncer(t, dst, dir)

	imported, err := dstSync.Import()
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if imported != 1 {
		t.Fatalf("imported %d, want 1", imported)
	}

	// El commit existe en el destino y su contenido viene redactado.
	c, err := dst.GetCommit("deadbeefcafe")
	if err != nil || c == nil {
		t.Fatalf("commit not imported: %v", err)
	}
	if strings.Contains(c.Snapshot, "hf_abcdefghij") {
		t.Errorf("secret leaked into imported snapshot: %s", c.Snapshot)
	}

	// Idempotencia: reimportar no duplica.
	again, err := dstSync.Import()
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Errorf("second import added %d, want 0", again)
	}
}

func mustSyncer(t *testing.T, store db.Store, dir string) *syncer {
	t.Helper()
	s, err := NewSyncer(store, WithDir(dir))
	if err != nil {
		t.Fatalf("NewSyncer: %v", err)
	}
	return s.(*syncer)
}
