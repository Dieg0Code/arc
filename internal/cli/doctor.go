package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/Dieg0Code/nem/internal/config"
	"github.com/spf13/cobra"
)

// newDoctorCmd crea `nem doctor [--fix]`: chequea las dependencias de las capas
// pro (Ollama/API + modelos) según la config, y con --fix las instala (en
// Windows vía scoop). Es la automatización del setup pro.
func newDoctorCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check (and with --fix install) the dependencies for pro features",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDoctor(cmd, fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "attempt to install/pull missing dependencies")
	return cmd
}

// requirement describe lo que un backend configurado necesita.
type requirement struct {
	layer    string // "summarize" | "embed"
	backend  string // "ollama" | "api"
	model    string
	endpoint string
}

func runDoctor(cmd *cobra.Command, fix bool) error {
	f, err := config.Load()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	reqs := collectReqs(f)
	if len(reqs) == 0 {
		fmt.Fprintln(out, "Pro features are OFF — nem runs on heuristic summaries + BM25 + structure (zero deps).")
		fmt.Fprintln(out, "Enable them with, e.g.:")
		fmt.Fprintln(out, "  nem config set embed.backend ollama")
		fmt.Fprintln(out, "  nem config set summarize.backend ollama")
		fmt.Fprintln(out, "then run: nem doctor --fix")
		return nil
	}

	ok := true
	tagsCache := map[string][]string{} // endpoint -> models (ollama)
	for _, r := range reqs {
		switch r.backend {
		case "ollama":
			ok = checkOllama(cmd, r, fix, tagsCache) && ok
		case "api":
			ok = checkAPI(cmd, r) && ok
		default:
			fmt.Fprintf(out, "✗ %s: unknown backend %q\n", r.layer, r.backend)
			ok = false
		}
	}
	if ok {
		fmt.Fprintln(out, "\nAll pro dependencies satisfied. Run 'nem index' to (re)build with them.")
	} else if !fix {
		fmt.Fprintln(out, "\nSome deps are missing. Re-run with --fix to install them automatically.")
	}
	return nil
}

func collectReqs(f *config.File) []requirement {
	var reqs []requirement
	add := func(layer string, b config.Backend) {
		if b.Backend == "" || b.Backend == "heuristic" || b.Backend == "none" {
			return
		}
		reqs = append(reqs, requirement{layer: layer, backend: b.Backend, model: b.Model, endpoint: b.Endpoint})
	}
	add("summarize", f.Summarize)
	add("embed", f.Embed)
	return reqs
}

func checkOllama(cmd *cobra.Command, r requirement, fix bool, cache map[string][]string) bool {
	out := cmd.OutOrStdout()
	endpoint := r.endpoint
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	model := r.model
	if model == "" {
		model = defaultModel(r) // mostrar el default que usará nem
	}

	models, cached := cache[endpoint]
	if !cached {
		var reachable bool
		models, reachable = ollamaTags(endpoint)
		if !reachable {
			models = nil
		}
		cache[endpoint] = models
	}
	if models == nil {
		fmt.Fprintf(out, "✗ %s (ollama): server not reachable at %s\n", r.layer, endpoint)
		if !hasCmd("ollama") {
			if fix {
				installOllama(cmd)
			} else {
				fmt.Fprintln(out, "    install:  scoop install ollama   (Windows)")
			}
		}
		fmt.Fprintln(out, "    start:    ollama serve   (leave it running), then re-run 'nem doctor --fix'")
		return false
	}

	fmt.Fprintf(out, "✓ %s (ollama): server up at %s\n", r.layer, endpoint)
	if modelPresent(models, model) {
		fmt.Fprintf(out, "  ✓ model %q present\n", model)
		return true
	}
	fmt.Fprintf(out, "  ✗ model %q not pulled\n", model)
	if fix {
		return pullModel(cmd, model)
	}
	fmt.Fprintf(out, "    pull:     ollama pull %s\n", model)
	return false
}

func checkAPI(cmd *cobra.Command, r requirement) bool {
	out := cmd.OutOrStdout()
	if os.Getenv("OPENAI_API_KEY") != "" {
		fmt.Fprintf(out, "✓ %s (api): OPENAI_API_KEY is set\n", r.layer)
		return true
	}
	fmt.Fprintf(out, "✗ %s (api): OPENAI_API_KEY not set\n", r.layer)
	fmt.Fprintln(out, "    fix:      set the OPENAI_API_KEY environment variable")
	return false
}

// --- helpers ---

func defaultModel(r requirement) string {
	if r.layer == "embed" {
		return "nomic-embed-text"
	}
	return "llama3.2"
}

func ollamaTags(endpoint string) ([]string, bool) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint + "/api/tags")
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()
	var r struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, false
	}
	names := make([]string, len(r.Models))
	for i, m := range r.Models {
		names[i] = m.Name
	}
	return names, true
}

// modelPresent matchea "llama3.2" contra "llama3.2:latest" etc.
func modelPresent(models []string, want string) bool {
	for _, m := range models {
		if m == want || strings.HasPrefix(m, want+":") || strings.TrimSuffix(m, ":latest") == want {
			return true
		}
	}
	return false
}

func hasCmd(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func installOllama(cmd *cobra.Command) {
	out := cmd.OutOrStdout()
	if runtime.GOOS != "windows" || !hasCmd("scoop") {
		fmt.Fprintln(out, "    (auto-install only supported via scoop on Windows; install Ollama manually: https://ollama.com)")
		return
	}
	fmt.Fprintln(out, "    → running: scoop install ollama")
	run(cmd, "scoop", "install", "ollama")
}

func pullModel(cmd *cobra.Command, model string) bool {
	out := cmd.OutOrStdout()
	if !hasCmd("ollama") {
		fmt.Fprintln(out, "    (ollama not on PATH; install it first)")
		return false
	}
	fmt.Fprintf(out, "    → running: ollama pull %s\n", model)
	return run(cmd, "ollama", "pull", model) == nil
}

func run(cmd *cobra.Command, name string, args ...string) error {
	c := exec.Command(name, args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	return c.Run()
}
