// Package scope resuelve el alcance de lectura activo: dado un scope con nombre
// (de config), calcula qué chats puede ver el agente. Es opt-in — sin scope
// activo, el acceso es completo (comportamiento por defecto de nem).
package scope

import (
	"fmt"
	"path"
	"slices"

	"github.com/Dieg0Code/nem/internal/config"
)

// ChatRef es la metadata mínima de un chat necesaria para resolver el scope.
type ChatRef struct {
	ID     string
	Title  string
	Source string
}

// Resolver decide qué chats son visibles bajo el scope activo.
type Resolver interface {
	// Active indica si hay un scope activo. Si es false, el acceso es completo.
	Active() bool
	// Name devuelve el nombre del scope activo ("" si no hay).
	Name() string
	// AllowedChatIDs filtra los chats dados a los que caen dentro del scope.
	// Si no hay scope activo, devuelve los ids de todos los chats.
	AllowedChatIDs(chats []ChatRef) ([]string, error)
}

type cfg struct {
	name   string
	scopes map[string]config.Scope
}

// Option configura el Resolver.
type Option func(*cfg) error

// WithName fija el nombre del scope activo. "" = sin scope (acceso completo).
func WithName(name string) Option {
	return func(c *cfg) error {
		c.name = name
		return nil
	}
}

// WithScopes inyecta los scopes definidos (normalmente de config.Scopes()).
func WithScopes(scopes map[string]config.Scope) Option {
	return func(c *cfg) error {
		c.scopes = scopes
		return nil
	}
}

type resolver struct {
	name    string
	active  bool
	scope   config.Scope
	allScps map[string]config.Scope
}

// New crea un Resolver. Si el nombre no está vacío pero no existe entre los
// scopes configurados, devuelve un error claro.
func New(options ...Option) (Resolver, error) {
	c := &cfg{}
	for _, option := range options {
		if err := option(c); err != nil {
			return nil, fmt.Errorf("failed to apply scope option: %w", err)
		}
	}
	r := &resolver{name: c.name, allScps: c.scopes}
	if c.name == "" {
		return r, nil // sin scope activo: acceso completo
	}
	sc, ok := c.scopes[c.name]
	if !ok {
		return nil, fmt.Errorf("unknown scope %q (define it in ~/.nem/config.toml)", c.name)
	}
	r.active = true
	r.scope = sc
	return r, nil
}

func (r *resolver) Active() bool { return r.active }
func (r *resolver) Name() string { return r.name }

// AllowedChatIDs devuelve los ids de los chats visibles. Sin scope activo, son
// todos; con scope activo, solo los que matchean.
func (r *resolver) AllowedChatIDs(chats []ChatRef) ([]string, error) {
	ids := make([]string, 0, len(chats))
	for _, c := range chats {
		if !r.active || matches(r.scope, c) {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

// matches indica si un chat cae dentro de un scope.
func matches(s config.Scope, c ChatRef) bool {
	// title-or-chat: si no hay filtros de título/chat, todo pasa este criterio.
	titleOrChat := len(s.Titles) == 0 && len(s.Chats) == 0
	if !titleOrChat {
		for _, pat := range s.Titles {
			if ok, _ := path.Match(pat, c.Title); ok {
				titleOrChat = true
				break
			}
		}
	}
	if !titleOrChat && slices.Contains(s.Chats, c.ID) {
		titleOrChat = true
	}
	if !titleOrChat {
		return false
	}
	// source: vacío = no filtra.
	return len(s.Sources) == 0 || slices.Contains(s.Sources, c.Source)
}
