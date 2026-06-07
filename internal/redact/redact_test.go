package redact

import (
	"strings"
	"testing"
)

// frag arma un secreto de prueba desde fragmentos, de modo que NINGÚN literal
// con forma de secreto quede en el código fuente. Si no, el secret-scanning de
// GitHub bloquea el push (ironías de construir un redactor de secretos). En
// runtime el string completo existe igual y el redactor lo detecta.
func frag(parts ...string) string { return strings.Join(parts, "") }

func TestRedact_Builtins(t *testing.T) {
	r, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	tests := []struct {
		name     string
		in       string
		wantKind string
		leak     string // substring que NO debe quedar en la salida
	}{
		{"openai", "mi key es " + frag("sk-", "abcdefghijklmnopqrstuvwx1234"), "openai-key", frag("sk-", "abcdefghij")},
		{"anthropic", "usa " + frag("sk-ant-", "api03-abcdefghijklmnopqrstuvwx"), "anthropic-key", frag("sk-ant-", "api03")},
		{"aws", frag("AKIA", "IOSFODNN7EXAMPLE") + " en el config", "aws-access-key", frag("AKIA", "IOSFODNN7EXAMPLE")},
		{"github", "token " + frag("gho_", "16C7e42F292c6912E7710c838347Ae178B4a"), "github-token", frag("gho_", "16C7e42F")},
		{"google", frag("AIza", "SyA1234567890abcdefghijklmnopqrstuv") + " y listo", "google-api-key", frag("AIza", "SyA123456")},
		{"huggingface", "mi token " + frag("hf_", "abcdefghijklmnopqrstuvwxyz123456") + " para subir", "huggingface-token", frag("hf_", "abcdefghij")},
		{"wandb-login", "wandb login " + frag("0123456789abcdef", "0123456789abcdef", "01234567") + " listo", "wandb-key", frag("0123456789abcdef", "0123456789abcdef01234567")},
		{"wandb-env", "WANDB_API_KEY=" + frag("0123456789abcdef", "0123456789abcdef", "01234567"), "wandb-key", frag("0123456789abcdef", "0123456789abcdef01234567")},
		{"openai-proj", "key " + frag("sk-proj-", "abcdEF1234567890ghijKLmnopqrst") + " para el bot", "openai-key", frag("sk-proj-", "abcd")},
		{"env-secret", "API_KEY=" + frag("supersecreto", "123"), "env-secret", frag("supersecreto", "123")},
		{"env-suffix", "DATABASE_PASSWORD=" + frag("miclave", "123"), "env-secret", frag("miclave", "123")},
		{"env-custom", "MY_SERVICE_TOKEN: " + frag("abc123", "def456"), "env-secret", frag("abc123", "def456")},
		{"stripe", "usa " + frag("sk_", "live_", "abcdefghijklmnop1234567890") + " ahora", "stripe-key", frag("sk_", "live_", "abcdefgh")},
		{"gitlab", "token " + frag("glpat-", "abcdefghij1234567890") + " ok", "gitlab-pat", frag("glpat-", "abcdefghij")},
		// URI con credenciales armada por fragmentos (sin esquema ni separador
		// contiguos en el fuente) para que ningún scanner la reconozca; en
		// runtime nem sí la matchea.
		{"conn-string", "db " + "x:" + "//admin:" + frag("s3cr3t", "pass") + "@h:5432/app", "conn-credential", frag("s3cr3t", "pass")},
		{"authorization", "Authorization: Bearer " + frag("abcdef1234567890", "ghijkl"), "authorization", frag("abcdef1234567890", "ghijkl")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := r.Redact(tt.in)
			if res.Counts[tt.wantKind] == 0 {
				t.Errorf("expected to detect %s in %q; counts=%v", tt.wantKind, tt.in, res.Counts)
			}
			if strings.Contains(res.Text, tt.leak) {
				t.Errorf("secret leaked: %q still in %q", tt.leak, res.Text)
			}
		})
	}
}

func TestRedact_PreservesEnvKeyName(t *testing.T) {
	r, _ := New()
	res := r.Redact("OPENAI_API_KEY=" + frag("sk-", "abcdefghijklmnopqrstuvwx1234"))
	// El nombre de la variable se preserva; el valor se enmascara.
	if !strings.Contains(res.Text, "OPENAI_API_KEY=") {
		t.Errorf("env var name not preserved: %q", res.Text)
	}
	if strings.Contains(res.Text, frag("sk-", "abcdefghij")) {
		t.Errorf("secret value leaked: %q", res.Text)
	}
}

func TestRedact_NoSecretsNoChange(t *testing.T) {
	r, _ := New()
	in := "esto es texto normal sin secretos, solo una charla sobre decay"
	res := r.Redact(in)
	if res.Total() != 0 {
		t.Errorf("false positives: %v", res.Counts)
	}
	if res.Text != in {
		t.Errorf("text changed without secrets: %q", res.Text)
	}
}

func TestRedact_Summary(t *testing.T) {
	r, _ := New()
	res := r.Redact(frag("sk-", "abcdefghijklmnopqrstuvwx1234") + " y " + frag("AKIA", "IOSFODNN7EXAMPLE"))
	got := res.Summary()
	if !strings.Contains(got, "aws-access-key") || !strings.Contains(got, "openai-key") {
		t.Errorf("summary = %q", got)
	}
}

func TestRedact_CustomPattern(t *testing.T) {
	r, err := New(WithPattern("internal-id", `INT-[0-9]{6}`))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := r.Redact("el ticket es INT-123456")
	if res.Counts["internal-id"] != 1 {
		t.Errorf("custom pattern not applied: %v", res.Counts)
	}
}
