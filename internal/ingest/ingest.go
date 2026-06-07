// Package ingest parsea los archivos de sesión de los agentes (Codex, Claude
// Code) y los persiste en el Store de arc. Los parsers son puros (archivo →
// ParsedChat); la orquestación inserta de forma idempotente.
package ingest

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Dieg0Code/arc/internal/db"
)

// Roles con los que se persisten los mensajes. user/assistant son la
// conversación; reasoning es el razonamiento del agente (thinking/reasoning);
// tool son las llamadas a herramientas y sus salidas (compactadas).
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleReasoning = "reasoning"
	RoleTool      = "tool"
)

// maxToolChars acota el tamaño del contenido relacionado a tools (argumentos y
// salidas). Evita inflar la DB y contaminar el índice FTS5 con vuelcos enormes
// de logs o archivos. El reasoning NO se trunca (es conciso y valioso).
const maxToolChars = 2000

// maxReasoningChars acota el razonamiento. Es más generoso que las tools (el
// razonamiento es la señal que arc más quiere preservar) pero igual bounded.
const maxReasoningChars = 4000

// truncate recorta s a max caracteres (runas), agregando una marca si se cortó.
func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n…[truncado]"
}

// ParsedChat es el resultado de parsear un archivo de sesión: el chat y sus
// mensajes, listos para persistir.
type ParsedChat struct {
	Chat     db.Chat
	Messages []db.Message
}

// Parser convierte un archivo de sesión de un agente en un ParsedChat. Es la
// abstracción que implementa cada agente soportado (codex, claude).
type Parser interface {
	// Source identifica al agente: "codex" | "claude".
	Source() string
	// DefaultRoot devuelve el directorio raíz por defecto de las sesiones.
	DefaultRoot() (string, error)
	// Parse parsea el contenido de un archivo de sesión.
	Parse(r io.Reader, sessionPath string) (*ParsedChat, error)
}

// Report resume el resultado de una corrida de ingesta.
type Report struct {
	Source   string
	Files    int
	Chats    int
	Messages int64
	Skipped  int
	Errors   []string
}

// config contiene las opciones de Ingest.
type config struct {
	root string
}

// Option configura una corrida de Ingest.
type Option func(*config) error

// WithRoot fuerza el directorio raíz de sesiones (en vez del default del parser).
func WithRoot(root string) Option {
	return func(c *config) error {
		if root == "" {
			return errors.New("root cannot be empty")
		}
		c.root = root
		return nil
	}
}

// Ingest recorre los archivos .jsonl bajo el root del parser, los parsea y los
// persiste en el store de forma idempotente (re-ingestar no duplica). Los
// errores por archivo no abortan la corrida: se acumulan en el Report.
func Ingest(store db.Store, p Parser, options ...Option) (*Report, error) {
	if store == nil {
		return nil, errors.New("store is required")
	}
	if p == nil {
		return nil, errors.New("parser is required")
	}

	cfg := &config{}
	for _, option := range options {
		if err := option(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply ingest option: %w", err)
		}
	}

	root := cfg.root
	if root == "" {
		r, err := p.DefaultRoot()
		if err != nil {
			return nil, err
		}
		root = r
	}

	files, err := sessionFiles(root)
	if err != nil {
		return nil, err
	}

	report := &Report{Source: p.Source()}
	for _, path := range files {
		report.Files++
		parsed, err := parseFile(p, path)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		if parsed == nil || len(parsed.Messages) == 0 {
			report.Skipped++
			continue
		}
		if err := store.UpsertChat(&parsed.Chat); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		n, err := store.InsertMessages(parsed.Messages)
		if err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		report.Chats++
		report.Messages += n
	}

	return report, nil
}

// parseFile abre y parsea un único archivo, garantizando el cierre.
func parseFile(p Parser, path string) (*ParsedChat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open session file: %w", err)
	}
	defer f.Close()
	return p.Parse(f, path)
}

// sessionFiles devuelve todos los .jsonl bajo root (recursivo). Si root no
// existe, devuelve lista vacía sin error (el agente puede no estar instalado).
func sessionFiles(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat root %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root %s is not a directory", root)
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // saltar entradas ilegibles, no abortar
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".jsonl") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk %s: %w", root, err)
	}
	return files, nil
}

// parseTimestamp convierte un timestamp ISO8601 a unix seconds. Devuelve 0 si
// no se puede parsear.
func parseTimestamp(s string) int64 {
	if s == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Unix()
		}
	}
	return 0
}

// titleFromCwd deriva un título legible del directorio de trabajo de la sesión,
// tolerando separadores de Windows y Unix.
func titleFromCwd(cwd string) string {
	if cwd == "" {
		return ""
	}
	norm := strings.ReplaceAll(cwd, "\\", "/")
	norm = strings.TrimRight(norm, "/")
	if idx := strings.LastIndex(norm, "/"); idx >= 0 {
		return norm[idx+1:]
	}
	return norm
}
