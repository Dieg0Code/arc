package index

import (
	"strings"
	"testing"

	"github.com/Dieg0Code/nem/internal/db"
)

func u(content string) db.Message { return db.Message{Role: "user", Content: content} }
func a(content string) db.Message { return db.Message{Role: "assistant", Content: content} }

func TestHeuristicSummary(t *testing.T) {
	tests := []struct {
		name       string
		title      string
		msgs       []db.Message
		wantSubstr []string // deben aparecer
		wantAbsent []string // NO deben aparecer
		wantExact  string    // si != "", igualdad exacta
	}{
		{
			name:  "skips AGENTS.md boilerplate, uses real request",
			title: "myproj",
			msgs: []db.Message{
				u("# AGENTS.md instructions for c:\\dev\\myproj\n<INSTRUCTIONS>\n## Propósito del repositorio\nblah blah\n</INSTRUCTIONS>"),
				u("Implementá el decay del learning rate en train.py para estabilizar el entrenamiento."),
			},
			wantSubstr: []string{"decay", "learning rate"},
			wantAbsent: []string{"AGENTS", "Propósito", "INSTRUCTIONS"},
		},
		{
			name:  "unwraps Codex IDE-context request",
			title: "nano-language-model",
			msgs: []db.Message{
				u("# Context from my IDE setup:\n## Active file: train.py\n## Open tabs:\n- train.py\n## My request for Codex:\nAgregá early stopping al bucle de entrenamiento del modelo."),
			},
			wantSubstr: []string{"early stopping", "entrenamiento"},
			wantAbsent: []string{"Active file", "Open tabs", "Context from my IDE"},
		},
		{
			name:  "skips continued-session summary",
			title: "arc",
			msgs: []db.Message{
				u("This session is being continued from a previous conversation that ran out of context. Summary: lots of stuff..."),
				u("Arreglá el bug del lock en el detector de sesión."),
			},
			wantSubstr: []string{"lock", "detector"},
			wantAbsent: []string{"continued", "previous conversation"},
		},
		{
			name:  "strips environment_context wrapper",
			title: "nano-language-model",
			msgs: []db.Message{
				u("<environment_context>\n<cwd>c:\\Users\\x\\nano-language-model</cwd>\n<shell>powershell</shell>\n</environment_context>\nCreá un modelo de lenguaje nano estilo GPT entrenado desde cero."),
			},
			wantSubstr: []string{"modelo de lenguaje", "nano"},
			wantAbsent: []string{"environment_context", "cwd", "powershell"},
		},
		{
			name:  "strips tool/image artifacts from lead and topics",
			title: "proj",
			msgs: []db.Message{
				u("arregla el archivo de power bi de esta ruta [Image #1] [external_agent_tool_call: Bash] [external_agent_tool_result]"),
			},
			wantSubstr: []string{"power bi"},
			wantAbsent: []string{"external_agent_tool", "Image #1"},
		},
		{
			name:  "skips /init slash command",
			title: "ataxx",
			msgs: []db.Message{
				u("/init"),
				u("Creá un parser para las sesiones de Codex en formato JSONL."),
			},
			wantSubstr: []string{"parser", "Codex"},
			wantAbsent: []string{"/init"},
		},
		{
			name:  "cleans markdown and code fences, adds topics tail",
			title: "myproj",
			msgs: []db.Message{
				u("## Plan\nNecesito **refactorizar** la interfaz `Parser` y agregar un retry al parser. El parser parser debe reintentar.\n```go\nfunc X(){}\n```"),
			},
			wantSubstr: []string{"refactorizar", "· topics:", "parser"},
			wantAbsent: []string{"```", "**", "## Plan", "func X"},
		},
		{
			name:  "strips Claude Code slash-command wrappers",
			title: "ai-lab",
			msgs: []db.Message{
				u("<command-message>init</command-message>\n<command-name>/init</command-name>\nPlease analyze this codebase and create a guide with the build commands."),
			},
			wantSubstr: []string{"analyze this codebase", "build commands"},
			wantAbsent: []string{"command-message", "command-name"},
		},
		{
			name:      "pure slash command falls back to title",
			title:     "geogreen",
			msgs:      []db.Message{u("<command-message>init</command-message> <command-name>/init</command-name>")},
			wantExact: "geogreen",
		},
		{
			name:  "falls back to assistant when all user msgs are boilerplate",
			title: "proj",
			msgs: []db.Message{
				u("/init"),
				a("Voy a generar el archivo de configuración con los comandos del repo."),
			},
			wantSubstr: []string{"configuración"},
		},
		{
			name:      "falls back to title when everything is boilerplate",
			title:     "ataxx-zero",
			msgs:      []db.Message{u("/init"), u("# AGENTS.md instructions for x")},
			wantExact: "ataxx-zero",
		},
		{
			name:      "empty chat",
			title:     "",
			msgs:      []db.Message{u("")},
			wantExact: "(empty chat)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HeuristicSummary(db.Chat{Title: tt.title}, tt.msgs)
			if tt.wantExact != "" && got != tt.wantExact {
				t.Fatalf("got %q, want exact %q", got, tt.wantExact)
			}
			for _, s := range tt.wantSubstr {
				if !strings.Contains(got, s) {
					t.Errorf("got %q, want substring %q", got, s)
				}
			}
			for _, s := range tt.wantAbsent {
				if strings.Contains(got, s) {
					t.Errorf("got %q, should NOT contain %q", got, s)
				}
			}
			if n := len([]rune(got)); n > maxSummaryChars {
				t.Errorf("summary too long: %d runes (max %d)", n, maxSummaryChars)
			}
		})
	}
}

func TestIsBoilerplate(t *testing.T) {
	yes := []string{
		"# AGENTS.md instructions for x", "Repository Guidelines\n=====",
		"this session is being continued from a previous", "/init", "INIT",
		"# Context from my IDE setup:\n...", "```go\nfunc(){}\n```", "  /resume  ",
	}
	no := []string{
		"Implementá el decay", "arregla el bug del lock", "Creá un CLI con cobra",
	}
	for _, s := range yes {
		if !isBoilerplate(s) {
			t.Errorf("isBoilerplate(%q) = false, want true", s)
		}
	}
	for _, s := range no {
		if isBoilerplate(s) {
			t.Errorf("isBoilerplate(%q) = true, want false", s)
		}
	}
}

func TestUnwrapRequest(t *testing.T) {
	in := "# Context from my IDE setup:\n## My request for Codex:\nhacé X"
	if got := unwrapRequest(in); got != "hacé X" {
		t.Errorf("unwrapRequest = %q, want %q", got, "hacé X")
	}
	in2 := "<system-reminder>noise</system-reminder>el pedido real"
	if got := unwrapRequest(in2); !strings.Contains(got, "el pedido real") || strings.Contains(got, "noise") {
		t.Errorf("unwrapRequest did not strip system-reminder: %q", got)
	}
	// Bloque tool-call de Codex con contenido entre marcadores.
	in3 := "[external_agent_tool_call: Bash] description: List dir command: ls foo [/external_agent_tool_call] hacé el refactor"
	if got := unwrapRequest(in3); !strings.Contains(got, "hacé el refactor") || strings.Contains(got, "external") || strings.Contains(got, "ls foo") {
		t.Errorf("unwrapRequest did not strip tool-call block: %q", got)
	}
}

func TestKeywords(t *testing.T) {
	msgs := []db.Message{u("el parser parser de codex lee jsonl y reintenta el parser")}
	got := keywords(msgs, "myproj", 5)
	if len(got) == 0 || got[0] != "parser" {
		t.Fatalf("keywords = %v, want 'parser' first (highest freq)", got)
	}
	for _, w := range got {
		if _, stop := stopwords[w]; stop {
			t.Errorf("keyword %q is a stopword", w)
		}
	}
}
