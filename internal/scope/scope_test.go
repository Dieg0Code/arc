package scope

import (
	"sort"
	"strings"
	"testing"

	"github.com/Dieg0Code/nem/internal/config"
)

var sampleChats = []ChatRef{
	{ID: "1", Title: "ataxx-zero", Source: "codex"},
	{ID: "2", Title: "ataxx-zero-ai", Source: "claude"},
	{ID: "3", Title: "pro301-taller-de-aplicaciones-para-internet", Source: "codex"},
	{ID: "4", Title: "aiep-subtitulos", Source: "codex"},
	{ID: "5", Title: "nano-language-model", Source: "claude"},
}

func allowedSorted(t *testing.T, r Resolver) []string {
	t.Helper()
	ids, err := r.AllowedChatIDs(sampleChats)
	if err != nil {
		t.Fatalf("AllowedChatIDs: %v", err)
	}
	sort.Strings(ids)
	return ids
}

func TestResolver_NoScopeIsFullAccess(t *testing.T) {
	r, err := New(WithName(""), WithScopes(map[string]config.Scope{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if r.Active() {
		t.Error("Active() = true, want false for empty name")
	}
	got := allowedSorted(t, r)
	if len(got) != len(sampleChats) {
		t.Errorf("got %v, want all %d chats", got, len(sampleChats))
	}
}

func TestResolver_UnknownScopeErrors(t *testing.T) {
	_, err := New(WithName("nope"), WithScopes(map[string]config.Scope{}))
	if err == nil {
		t.Fatal("expected error for unknown scope")
	}
}

func TestResolver_Matching(t *testing.T) {
	scopes := map[string]config.Scope{
		"ataxx":    {Titles: []string{"ataxx-zero", "ataxx-zero-ai"}},
		"teaching": {Titles: []string{"pro301*", "aiep*"}},
		"codexers": {Sources: []string{"codex"}},
		"teach-cx": {Titles: []string{"pro301*", "aiep*"}, Sources: []string{"codex"}},
		"byid":     {Chats: []string{"5"}},
		"mix":      {Titles: []string{"ataxx-zero"}, Chats: []string{"3"}},
	}

	tests := []struct {
		name  string
		scope string
		want  []string
	}{
		{"exact titles", "ataxx", []string{"1", "2"}},
		{"glob titles", "teaching", []string{"3", "4"}},
		{"source only", "codexers", []string{"1", "3", "4"}},
		{"title glob AND source", "teach-cx", []string{"3", "4"}},
		{"explicit chat id", "byid", []string{"5"}},
		{"title or explicit id", "mix", []string{"1", "3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := New(WithName(tt.scope), WithScopes(scopes))
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if !r.Active() {
				t.Error("Active() = false, want true")
			}
			got := allowedSorted(t, r)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("scope %q allowed %v, want %v", tt.scope, got, tt.want)
			}
		})
	}
}
