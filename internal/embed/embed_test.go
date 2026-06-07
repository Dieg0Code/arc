package embed

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEncodeDecodeRoundtrip(t *testing.T) {
	v := []float32{0.1, -2.5, 3.0, 0, 42.42}
	got := Decode(Encode(v))
	if len(got) != len(v) {
		t.Fatalf("len = %d, want %d", len(got), len(v))
	}
	for i := range v {
		if got[i] != v[i] {
			t.Errorf("pos %d: got %v, want %v", i, got[i], v[i])
		}
	}
}

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 0}
	if c := Cosine(a, a); math.Abs(c-1) > 1e-6 {
		t.Errorf("identical: got %v, want 1", c)
	}
	if c := Cosine([]float32{1, 0}, []float32{0, 1}); math.Abs(c) > 1e-6 {
		t.Errorf("orthogonal: got %v, want 0", c)
	}
	if c := Cosine([]float32{1, 2}, []float32{1, 2, 3}); c != 0 {
		t.Errorf("mismatched dims: got %v, want 0", c)
	}
}

func TestNew_Validation(t *testing.T) {
	if _, err := New("bogus"); err == nil {
		t.Error("expected error for unknown backend")
	}
	t.Setenv("OPENAI_API_KEY", "")
	if _, err := New("api"); err == nil {
		t.Error("expected error: api without key")
	}
}

func TestOllama_Stub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"embedding":[0.1,0.2,0.3]}`))
	}))
	defer srv.Close()

	e, err := New("ollama", WithEndpoint(srv.URL), WithModel("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	vecs, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vecs) != 2 || len(vecs[0]) != 3 {
		t.Fatalf("got %d vecs of dim %d", len(vecs), len(vecs[0]))
	}
}
