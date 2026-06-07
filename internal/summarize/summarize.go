// Package summarize genera resúmenes de nodos del índice con un backend
// pluggable. El default de nem es heurístico (en internal/index, sin deps);
// acá viven los backends con LLM —Ollama (local) y una API compatible con
// OpenAI— para resúmenes ricos cuando el usuario los configura. Solo usa
// net/http: nem sigue Go puro.
package summarize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	appconfig "github.com/Dieg0Code/nem/internal/config"
)

// promptTmpl instruye al LLM a resumir una sesión en 1-2 frases.
const promptTmpl = `You summarize an AI coding/work session in ONE or TWO sentences: the task and what was decided or built. No preamble, no markdown.

Title: %s

Conversation:
%s`

// Summarizer produce el resumen de una sesión a partir de su texto.
type Summarizer interface {
	Summarize(ctx context.Context, title, text string) (string, error)
}

type config struct {
	backend  string
	model    string
	endpoint string
	apiKey   string
	client   *http.Client
}

// Option configura el Summarizer.
type Option func(*config) error

// WithModel fija el modelo (p.ej. "llama3.2" en Ollama, "gpt-4o-mini" en API).
func WithModel(m string) Option {
	return func(c *config) error { c.model = m; return nil }
}

// WithEndpoint fija el endpoint base del backend.
func WithEndpoint(e string) Option {
	return func(c *config) error { c.endpoint = e; return nil }
}

// WithAPIKey fija la API key (backend "api"). Si no, se usa OPENAI_API_KEY.
func WithAPIKey(k string) Option {
	return func(c *config) error { c.apiKey = k; return nil }
}

// New crea un Summarizer para el backend dado: "ollama" | "api".
func New(backend string, options ...Option) (Summarizer, error) {
	cfg := &config{backend: backend, client: &http.Client{Timeout: 90 * time.Second}}
	for _, o := range options {
		if err := o(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply summarize option: %w", err)
		}
	}
	switch backend {
	case "ollama":
		if cfg.endpoint == "" {
			cfg.endpoint = "http://localhost:11434"
		}
		if cfg.model == "" {
			cfg.model = "llama3.2"
		}
		return &ollama{cfg: cfg}, nil
	case "api":
		if cfg.endpoint == "" {
			cfg.endpoint = "https://api.openai.com/v1"
		}
		if cfg.model == "" {
			cfg.model = "gpt-4o-mini"
		}
		if cfg.apiKey == "" {
			cfg.apiKey = os.Getenv("OPENAI_API_KEY")
		}
		if cfg.apiKey == "" {
			return nil, errors.New("api backend needs an API key (WithAPIKey or OPENAI_API_KEY)")
		}
		return &openaiAPI{cfg: cfg}, nil
	default:
		return nil, fmt.Errorf("unknown summarize backend %q (use 'ollama' or 'api')", backend)
	}
}

// FromConfig construye un Summarizer según ~/.nem/config.toml [summarize].
// Devuelve (nil, nil) si el backend es vacío o "heuristic" (= usar el default
// heurístico del índice).
func FromConfig() (Summarizer, error) {
	f, err := appconfig.Load()
	if err != nil {
		return nil, err
	}
	b := f.Summarize
	if b.Backend == "" || b.Backend == "heuristic" {
		return nil, nil
	}
	return New(b.Backend, WithModel(b.Model), WithEndpoint(b.Endpoint))
}

func prompt(title, text string) string {
	return fmt.Sprintf(promptTmpl, strings.TrimSpace(title), strings.TrimSpace(text))
}

// --- Ollama ---

type ollama struct{ cfg *config }

func (o *ollama) Summarize(ctx context.Context, title, text string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":  o.cfg.model,
		"prompt": prompt(title, text),
		"stream": false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.cfg.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := o.cfg.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("ollama decode: %w", err)
	}
	return strings.TrimSpace(out.Response), nil
}

// --- OpenAI-compatible API ---

type openaiAPI struct{ cfg *config }

func (a *openaiAPI) Summarize(ctx context.Context, title, text string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":       a.cfg.model,
		"temperature": 0.2,
		"messages": []map[string]string{
			{"role": "user", "content": prompt(title, text)},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.cfg.apiKey)
	resp, err := a.cfg.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("api request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("api status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("api decode: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", errors.New("api returned no choices")
	}
	return strings.TrimSpace(out.Choices[0].Message.Content), nil
}
