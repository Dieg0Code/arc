// Package embed es la capa OPCIONAL de embeddings de nem (apagada por default).
// La base de la búsqueda es estructura + BM25 + razonamiento del agente; los
// embeddings agregan una señal semántica (para paráfrasis/idioma) que se fusiona
// por RRF. Backends pluggables Ollama (local) y API (OpenAI-compatible), solo
// net/http: nem sigue Go puro. Se embeben los resúmenes de los nodos del índice
// (baratos: decenas), no los 97k mensajes.
package embed

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"time"

	appconfig "github.com/Dieg0Code/nem/internal/config"
)

// Embedder convierte textos en vectores. Embed procesa un lote.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type config struct {
	model    string
	endpoint string
	apiKey   string
	client   *http.Client
}

// Option configura el Embedder.
type Option func(*config) error

// WithModel fija el modelo de embeddings.
func WithModel(m string) Option { return func(c *config) error { c.model = m; return nil } }

// WithEndpoint fija el endpoint base.
func WithEndpoint(e string) Option { return func(c *config) error { c.endpoint = e; return nil } }

// WithAPIKey fija la API key (backend "api"; si no, OPENAI_API_KEY).
func WithAPIKey(k string) Option { return func(c *config) error { c.apiKey = k; return nil } }

// New crea un Embedder para "ollama" | "api".
func New(backend string, options ...Option) (Embedder, error) {
	cfg := &config{client: &http.Client{Timeout: 60 * time.Second}}
	for _, o := range options {
		if err := o(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply embed option: %w", err)
		}
	}
	switch backend {
	case "ollama":
		if cfg.endpoint == "" {
			cfg.endpoint = "http://localhost:11434"
		}
		if cfg.model == "" {
			cfg.model = "nomic-embed-text"
		}
		return &ollama{cfg: cfg}, nil
	case "api":
		if cfg.endpoint == "" {
			cfg.endpoint = "https://api.openai.com/v1"
		}
		if cfg.model == "" {
			cfg.model = "text-embedding-3-small"
		}
		if cfg.apiKey == "" {
			cfg.apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if cfg.apiKey == "" {
			return nil, errors.New("api backend needs an API key (WithAPIKey or OPENAI_API_KEY)")
		}
		return &openaiAPI{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown embed backend %q (use 'ollama' or 'api')", backend)
	}
}

// FromConfig construye un Embedder según ~/.nem/config.toml [embed]. Devuelve
// (nil, nil) si el backend está vacío (= embeddings apagados).
func FromConfig() (Embedder, error) {
	f, err := appconfig.Load()
	if err != nil {
		return nil, err
	}
	b := f.Embed
	if b.Backend == "" || b.Backend == "none" {
		return nil, nil
	}
	return New(b.Backend, WithModel(b.Model), WithEndpoint(b.Endpoint))
}

// --- Ollama (un texto por request: /api/embeddings) ---

type ollama struct{ cfg *config }

func (o *ollama) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, 0, len(texts))
	for _, t := range texts {
		body, _ := json.Marshal(map[string]any{"model": o.cfg.model, "prompt": t})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.endpoint+"/api/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := o.cfg.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama embed: %w", err)
		}
		var r struct {
			Embedding []float32 `json:"embedding"`
		}
		err = json.NewDecoder(resp.Body).Decode(&r)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("ollama embed decode: %w", err)
		}
		out = append(out, r.Embedding)
	}
	return out, nil
}

// --- OpenAI-compatible API (lote: /embeddings) ---

type openaiAPI struct{ cfg *config }

func (a *openaiAPI) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": a.cfg.model, "input": texts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.endpoint+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.apiKey)
	resp, err := a.cfg.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api embed: %w", err)
	}
	defer resp.Body.Close()
	var r struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("api embed decode: %w", err)
	}
	out := make([][]float32, len(r.Data))
	for i, d := range r.Data {
		out[i] = d.Embedding
	}
	return out, nil
}

// --- vectores: cosine + (de)serialización a BLOB ---

// Cosine devuelve la similitud coseno de dos vectores (0 si dimensiones != ).
func Cosine(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / (math.Sqrt(na) * math.Sqrt(nb))
}

// Encode serializa un vector float32 a bytes (little-endian) para guardar en BLOB.
func Encode(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

// Decode deserializa un BLOB de Encode a un vector float32.
func Decode(b []byte) []float32 {
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}
