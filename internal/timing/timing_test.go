package timing

import (
	"testing"
	"time"
)

const base = int64(1_700_000_000)

func ev(role string, off int64) Event { return Event{Role: role, Ts: base + off} }

func TestCompute(t *testing.T) {
	tests := []struct {
		name       string
		evs        []Event
		wantActive int64 // segundos
		wantSess   int
		wantMsgs   int
		wantWall   int64 // segundos
	}{
		{"empty", nil, 0, 0, 0, 0},
		{"single", []Event{ev("user", 0)}, 0, 1, 1, 0},
		{"work gap under cap", []Event{ev("user", 0), ev("assistant", 600)}, 600, 1, 2, 600},
		{"work gap capped (busy agent)", []Event{ev("user", 0), ev("assistant", 3600)}, 1800, 2, 2, 3600},
		{"wait gap capped (user away)", []Event{ev("assistant", 0), ev("user", 3600)}, 300, 2, 2, 3600},
		{"wait gap under cap", []Event{ev("assistant", 0), ev("user", 120)}, 120, 1, 2, 120},
		{"filters zero-ts", []Event{ev("user", 0), {Role: "assistant", Ts: 0}, ev("user", 120)}, 120, 1, 2, 120},
		{"assistant->tool is work", []Event{ev("assistant", 0), ev("tool", 3600)}, 1800, 2, 2, 3600},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Compute(tt.evs)
			if int64(s.Active.Seconds()) != tt.wantActive {
				t.Errorf("Active = %v, want %ds", s.Active, tt.wantActive)
			}
			if s.Sessions != tt.wantSess {
				t.Errorf("Sessions = %d, want %d", s.Sessions, tt.wantSess)
			}
			if s.Msgs != tt.wantMsgs {
				t.Errorf("Msgs = %d, want %d", s.Msgs, tt.wantMsgs)
			}
			if int64(s.Wall.Seconds()) != tt.wantWall {
				t.Errorf("Wall = %v, want %ds", s.Wall, tt.wantWall)
			}
		})
	}
}

func TestMerge(t *testing.T) {
	a := Compute([]Event{ev("user", 0), ev("assistant", 600)})       // active 600, first base, last base+600
	b := Compute([]Event{ev("user", 10000), ev("assistant", 10600)}) // active 600, later
	m := a.Merge(b)
	if int64(m.Active.Seconds()) != 1200 {
		t.Errorf("merged Active = %v, want 1200s", m.Active)
	}
	if m.Sessions != 2 || m.Msgs != 4 {
		t.Errorf("merged Sessions=%d Msgs=%d, want 2/4", m.Sessions, m.Msgs)
	}
	if m.First != base || m.Last != base+10600 {
		t.Errorf("merged span [%d,%d], want [%d,%d]", m.First, m.Last, base, base+10600)
	}
	// Merge con vacío es identidad.
	if got := a.Merge(Span{}); got.Active != a.Active || got.Msgs != a.Msgs {
		t.Errorf("Merge with empty changed the span")
	}
}

func TestFormat(t *testing.T) {
	cases := map[time.Duration]string{
		0:                "0m",
		90 * time.Second: "1m",
		60 * time.Minute: "1h",
		61 * time.Minute: "1h1m",
		24 * time.Hour:   "1d",
		25 * time.Hour:   "1d 1h",
	}
	for d, want := range cases {
		if got := Format(d); got != want {
			t.Errorf("Format(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := base + 100000
	if got := Ago(0, now); got != "nunca" {
		t.Errorf("Ago(0) = %q, want nunca", got)
	}
	if got := Ago(now, now); got != "recién" {
		t.Errorf("Ago(now) = %q, want recién", got)
	}
	if got := Ago(now-3*86400, now); got != "hace 3d" {
		t.Errorf("Ago(3d) = %q, want 'hace 3d'", got)
	}
}
