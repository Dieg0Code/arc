package summarize

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_Validation(t *testing.T) {
	if _, err := New("bogus"); err == nil {
		t.Error("expected error for unknown backend")
	}
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := New("api"); err == nil {
		t.Error("expected error: api backend without key")
	}
}

func TestOllama_Stub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/api/generate") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"response":"  a concise summary  "}`))
	}))
	defer srv.Close()

	s, err := New("ollama", WithEndpoint(srv.URL), WithModel("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Summarize(context.Background(), "title", "some conversation")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got != "a concise summary" {
		t.Errorf("got %q, want trimmed summary", got)
	}
}

func TestAPI_Stub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing/!= auth header: %q", r.Header.Get("Authorization"))
		}
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"api summary"}}]}`))
	}))
	defer srv.Close()

	s, err := New("api", WithEndpoint(srv.URL), WithAPIKey("k"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := s.Summarize(context.Background(), "t", "text")
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got != "api summary" {
		t.Errorf("got %q", got)
	}
}
