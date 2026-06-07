// Package redact detecta y enmascara secretos (API keys, tokens, claves
// privadas) antes de que el contenido salga de la máquina vía `arc sync`. Es
// secure-by-default: el scrubbing corre siempre en la exportación a git.
package redact

import (
	"fmt"
	"regexp"
	"sort"
)

// rule es un detector de secretos con nombre.
type rule struct {
	kind string
	re   *regexp.Regexp
	// repl, si no es nil, controla el reemplazo (para preservar prefijos como
	// "KEY="); si es nil se reemplaza todo el match por [REDACTED:kind].
	repl func(string) string
}

// Result es el texto saneado más el conteo de secretos por tipo.
type Result struct {
	Text   string
	Counts map[string]int
}

// Total devuelve la cantidad total de secretos redactados.
func (r Result) Total() int {
	n := 0
	for _, c := range r.Counts {
		n += c
	}
	return n
}

// Summary devuelve un resumen ordenado tipo "2 openai-key, 1 aws-key".
func (r Result) Summary() string {
	if len(r.Counts) == 0 {
		return ""
	}
	kinds := make([]string, 0, len(r.Counts))
	for k := range r.Counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	out := ""
	for i, k := range kinds {
		if i > 0 {
			out += ", "
		}
		out += fmt.Sprintf("%d %s", r.Counts[k], k)
	}
	return out
}

// Redactor enmascara secretos en texto.
type Redactor interface {
	// Redact devuelve el texto saneado y el conteo de secretos encontrados.
	Redact(s string) Result
}

type redactor struct {
	rules []rule
}

type config struct {
	extra        []rule
	disableBuilt bool
}

// Option configura el Redactor.
type Option func(*config) error

// WithPattern agrega un detector propio (kind + regex). Útil para secretos
// específicos de un usuario/empresa.
func WithPattern(kind, pattern string) Option {
	return func(c *config) error {
		if kind == "" {
			return fmt.Errorf("kind cannot be empty")
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Errorf("invalid pattern for %s: %w", kind, err)
		}
		c.extra = append(c.extra, rule{kind: kind, re: re})
		return nil
	}
}

// WithoutBuiltins desactiva los detectores por defecto (solo se usan los propios).
func WithoutBuiltins() Option {
	return func(c *config) error {
		c.disableBuilt = true
		return nil
	}
}

// New crea un Redactor con los detectores por defecto más los configurados.
func New(options ...Option) (Redactor, error) {
	cfg := &config{}
	for _, option := range options {
		if err := option(cfg); err != nil {
			return nil, fmt.Errorf("failed to apply redact option: %w", err)
		}
	}
	var rules []rule
	if !cfg.disableBuilt {
		rules = append(rules, builtinRules()...)
	}
	rules = append(rules, cfg.extra...)
	return &redactor{rules: rules}, nil
}

// Redact aplica todos los detectores en orden. El orden importa: los más
// específicos (sk-ant- antes que sk-) van primero para no romper el match.
func (r *redactor) Redact(s string) Result {
	counts := map[string]int{}
	for _, ru := range r.rules {
		placeholder := "[REDACTED:" + ru.kind + "]"
		s = ru.re.ReplaceAllStringFunc(s, func(match string) string {
			counts[ru.kind]++
			if ru.repl != nil {
				return ru.repl(match)
			}
			return placeholder
		})
	}
	// Limpiar tipos con conteo 0 (no deberían existir, pero por las dudas).
	for k, v := range counts {
		if v == 0 {
			delete(counts, k)
		}
	}
	return Result{Text: s, Counts: counts}
}

// builtinRules es el set de detectores por defecto, ordenado de más específico
// a más general. Como el scrubbing corre en la frontera de salida y el modo es
// "redacta y reporta", se prefiere sobre-redactar antes que filtrar.
func builtinRules() []rule {
	mk := func(kind, pat string) rule {
		return rule{kind: kind, re: regexp.MustCompile(pat)}
	}
	// keep arma una regla que preserva un prefijo (grupos del template repl) y
	// enmascara solo la parte sensible.
	keep := func(kind, pat, repl string) rule {
		re := regexp.MustCompile(pat)
		return rule{kind: kind, re: re, repl: func(m string) string {
			return re.ReplaceAllString(m, repl)
		}}
	}

	return []rule{
		// --- bloques y formatos inequívocos ---
		mk("private-key", `-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`),
		mk("jwt", `eyJ[A-Za-z0-9_\-]{10,}\.eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}`),

		// --- proveedores con prefijo conocido (bajo falso-positivo) ---
		mk("anthropic-key", `sk-ant-[A-Za-z0-9_\-]{20,}`),
		mk("openai-key", `sk-(?:proj-|svcacct-|admin-)?[A-Za-z0-9_\-]{20,}`),
		mk("huggingface-token", `hf_[A-Za-z0-9]{30,}`),
		mk("aws-access-key", `(?:AKIA|ASIA|AGPA|AIDA|AROA|ANPA)[0-9A-Z]{16}`),
		mk("github-pat", `github_pat_[A-Za-z0-9_]{60,}`),
		mk("github-token", `gh[opsu]_[A-Za-z0-9]{36,}`),
		mk("gitlab-pat", `glpat-[A-Za-z0-9_\-]{20,}`),
		mk("google-api-key", `AIza[0-9A-Za-z_\-]{35}`),
		mk("google-oauth-secret", `GOCSPX-[A-Za-z0-9_\-]{20,}`),
		mk("slack-token", `xox[baprs]-[A-Za-z0-9-]{10,}`),
		mk("stripe-key", `(?:sk|rk|pk)_(?:live|test)_[A-Za-z0-9]{16,}`),
		mk("sendgrid-key", `SG\.[A-Za-z0-9_\-]{16,}\.[A-Za-z0-9_\-]{16,}`),
		mk("npm-token", `npm_[A-Za-z0-9]{36}`),
		mk("pypi-token", `pypi-[A-Za-z0-9_\-]{16,}`),
		mk("digitalocean-token", `dop_v1_[a-f0-9]{64}`),
		mk("telegram-bot-token", `\b\d{8,10}:[A-Za-z0-9_\-]{35}\b`),

		// --- contextuales (preservan el prefijo, enmascaran el valor) ---
		// connection string con credenciales: scheme://user:PASS@host
		keep("conn-credential", `([a-zA-Z][a-zA-Z0-9+.\-]*://[^:@/\s]+:)([^@/\s]{3,})(@)`, "${1}[REDACTED:conn-credential]${3}"),
		// header Authorization: Bearer/Basic/Token <valor>
		keep("authorization", `(?i)(authorization\s*[:=]\s*(?:bearer|basic|token)\s+)([A-Za-z0-9._\-+/=]{8,})`, "${1}[REDACTED:authorization]"),
		// bearer token suelto
		keep("bearer-token", `(?i)(bearer\s+)([A-Za-z0-9._\-]{20,})`, "${1}[REDACTED:bearer-token]"),
		// wandb: API key de 40 hex, solo cerca de contexto "wandb"
		keep("wandb-key", `(?i)(wandb[a-z0-9_.\s"'():=/-]{0,40}?)([0-9a-f]{40})`, "${1}[REDACTED:wandb-key]"),

		// --- env vars / asignaciones sensibles (el caso más común en inputs) ---
		// Cualquier nombre que CONTENGA una palabra sensible, = o : y un valor.
		// Excluye '[' del valor para no re-envolver placeholders ya redactados.
		keep("env-secret",
			`(?i)([A-Z0-9_.]*(?:secret|token|password|passwd|pwd|api[_-]?key|apikey|access[_-]?key|access[_-]?token|refresh[_-]?token|auth|credential|client[_-]?secret|private[_-]?key|passphrase)[A-Z0-9_.]*\s*[:=]\s*['"]?)([^\s'"\[\]]{6,})`,
			"${1}[REDACTED:env-secret]"),
	}
}
