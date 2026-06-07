package config

import (
	"bytes"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Scope define un alcance de acceso de lectura (ver internal/scope).
type Scope struct {
	Titles  []string `toml:"titles,omitempty"`
	Sources []string `toml:"sources,omitempty"`
	Chats   []string `toml:"chats,omitempty"`
}

// Backend configura un backend pluggable (summarize/embed): "heuristic" |
// "ollama" | "api" | "static", con modelo y endpoint opcionales.
type Backend struct {
	Backend  string `toml:"backend,omitempty"`
	Model    string `toml:"model,omitempty"`
	Endpoint string `toml:"endpoint,omitempty"`
}

// File es el contenido completo de ~/.nem/config.toml.
type File struct {
	Scopes    map[string]Scope `toml:"scopes,omitempty"`
	Summarize Backend          `toml:"summarize,omitempty"`
	Embed     Backend          `toml:"embed,omitempty"`
}

// Load lee config.toml. Si no existe, devuelve un File vacío (sin error).
func Load() (*File, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{}, nil
		}
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}
	var f File
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	return &f, nil
}

// Save escribe el File a config.toml (sobrescribe; los comentarios se pierden).
func Save(f *File) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(f); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("failed to write config %s: %w", path, err)
	}
	return nil
}

// Scopes devuelve los scopes configurados (mapa vacío si no hay).
func Scopes() (map[string]Scope, error) {
	f, err := Load()
	if err != nil {
		return nil, err
	}
	if f.Scopes == nil {
		return map[string]Scope{}, nil
	}
	return f.Scopes, nil
}

// settableKeys son las claves que `nem config set/get` maneja.
var settableKeys = map[string]bool{
	"summarize.backend": true, "summarize.model": true, "summarize.endpoint": true,
	"embed.backend": true, "embed.model": true, "embed.endpoint": true,
}

// Get devuelve el valor de una clave punteada (p.ej. "summarize.backend").
func Get(key string) (string, error) {
	f, err := Load()
	if err != nil {
		return "", err
	}
	switch key {
	case "summarize.backend":
		return f.Summarize.Backend, nil
	case "summarize.model":
		return f.Summarize.Model, nil
	case "summarize.endpoint":
		return f.Summarize.Endpoint, nil
	case "embed.backend":
		return f.Embed.Backend, nil
	case "embed.model":
		return f.Embed.Model, nil
	case "embed.endpoint":
		return f.Embed.Endpoint, nil
	default:
		return "", fmt.Errorf("unknown config key %q", key)
	}
}

// Set fija una clave punteada y persiste el archivo.
func Set(key, value string) error {
	if !settableKeys[key] {
		return fmt.Errorf("unknown config key %q (valid: summarize.backend/model/endpoint, embed.backend/model/endpoint)", key)
	}
	f, err := Load()
	if err != nil {
		return err
	}
	switch key {
	case "summarize.backend":
		f.Summarize.Backend = value
	case "summarize.model":
		f.Summarize.Model = value
	case "summarize.endpoint":
		f.Summarize.Endpoint = value
	case "embed.backend":
		f.Embed.Backend = value
	case "embed.model":
		f.Embed.Model = value
	case "embed.endpoint":
		f.Embed.Endpoint = value
	}
	return Save(f)
}

// Keys devuelve las claves configurables (para `nem config list`).
func Keys() []string {
	return []string{
		"summarize.backend", "summarize.model", "summarize.endpoint",
		"embed.backend", "embed.model", "embed.endpoint",
	}
}
