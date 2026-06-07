// Package timing mide la duración REAL de una conversación: el "tiempo activo"
// (trabajo efectivo) frente al span de calendario. La clave es que un agente no
// estima en unidades de equipo humano sin calibrar; con esto puede anclar sus
// estimaciones en cuánto tardó de verdad un trabajo análogo.
//
// Modelo role-aware: se recorren los mensajes en orden cronológico y cada hueco
// entre dos se clasifica por quién habló último —si el agente terminó y esperaba
// al humano (assistant→user) el hueco se topea corto (te fuiste); cualquier otro
// hueco es trabajo del agente y se topea generoso. Así un rato largo en que el
// agente está ocupado cuenta como activo (sus tool calls/results lo pueblan), y
// solo las ausencias reales del usuario se descuentan.
package timing

import (
	"fmt"
	"sort"
	"time"
)

// Topes por hueco (cap del modelo role-aware). Ajustables.
const (
	// WorkCap: máximo que aporta un hueco de trabajo (agente ocupado), por si fue
	// una sola generación larga sin pasos intermedios timestampeados.
	WorkCap = 30 * time.Minute
	// WaitCap: máximo que aporta un hueco de espera (agente terminó, espera al
	// humano); más que esto = el usuario se fue.
	WaitCap = 5 * time.Minute
	// SessionGap: hueco a partir del cual contamos una "sentada" nueva (volviste
	// más tarde). Independiente de los caps de tiempo activo.
	SessionGap = 45 * time.Minute
)

// Event es un mensaje reducido a (rol, timestamp) para medir duración.
type Event struct {
	Role string
	Ts   int64 // unix seconds
}

// Span resume la duración de una conversación (o agregado de varias).
type Span struct {
	Active   time.Duration // tiempo activo (trabajo real), role-aware
	Wall     time.Duration // span de calendario (último - primer mensaje)
	Sessions int           // nº de sentadas (huecos que superan su cap, +1)
	Msgs     int           // nº de mensajes con timestamp válido
	First    int64         // unix del primer mensaje
	Last     int64         // unix del último mensaje
}

// Compute calcula el Span de una secuencia de eventos en orden cronológico
// (orden de Seq). Filtra timestamps inválidos (<=0) preservando el orden.
func Compute(evs []Event) Span {
	v := make([]Event, 0, len(evs))
	for _, e := range evs {
		if e.Ts > 0 {
			v = append(v, e)
		}
	}
	if len(v) == 0 {
		return Span{}
	}
	// Ordenar por timestamp: los timestamps por mensaje no siempre son monótonos
	// por Seq (chats compactados), y ordenar garantiza huecos >=0 y active <= wall.
	sort.SliceStable(v, func(i, j int) bool { return v[i].Ts < v[j].Ts })

	sp := Span{First: v[0].Ts, Last: v[len(v)-1].Ts, Msgs: len(v), Sessions: 1}
	workCap := int64(WorkCap / time.Second)
	waitCap := int64(WaitCap / time.Second)
	sessionGap := int64(SessionGap / time.Second)
	for i := 1; i < len(v); i++ {
		gap := v[i].Ts - v[i-1].Ts // >=0 (ordenado)
		if gap > sessionGap {
			sp.Sessions++ // volviste más tarde = nueva sentada
		}
		capSec := workCap
		if v[i-1].Role == "assistant" && v[i].Role == "user" {
			capSec = waitCap // el agente terminó y esperaba al humano
		}
		sp.Active += time.Duration(min(gap, capSec)) * time.Second
	}
	sp.Wall = time.Duration(sp.Last-sp.First) * time.Second
	return sp
}

// Merge agrega dos spans (p.ej. los chats de un proyecto): el tiempo activo y las
// sesiones suman; el span de calendario abarca de la primera a la última.
func (s Span) Merge(o Span) Span {
	if o.Msgs == 0 {
		return s
	}
	if s.Msgs == 0 {
		return o
	}
	r := Span{
		Active:   s.Active + o.Active,
		Sessions: s.Sessions + o.Sessions,
		Msgs:     s.Msgs + o.Msgs,
		First:    min(s.First, o.First),
		Last:     max(s.Last, o.Last),
	}
	if wall := r.Last - r.First; wall > 0 {
		r.Wall = time.Duration(wall) * time.Second
	}
	return r
}

// Line es un resumen de una línea de la Span (para timeline/stats/nem_duration).
func (s Span) Line(now int64) string {
	if s.Msgs == 0 {
		return "no timestamped activity"
	}
	return fmt.Sprintf("active ~%s · calendar %s · %d session(s) · last %s",
		Format(s.Active), Format(s.Wall), s.Sessions, Ago(s.Last, now))
}

// Format imprime una duración de forma compacta: "45m", "2h10m", "3d 4h".
func Format(d time.Duration) string {
	if d <= 0 {
		return "0m"
	}
	totalMin := int64(d.Minutes())
	days := totalMin / (60 * 24)
	hours := (totalMin % (60 * 24)) / 60
	mins := totalMin % 60
	switch {
	case days > 0 && hours > 0:
		return fmt.Sprintf("%dd %dh", days, hours)
	case days > 0:
		return fmt.Sprintf("%dd", days)
	case hours > 0 && mins > 0:
		return fmt.Sprintf("%dh%dm", hours, mins)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", mins)
	}
}

// Ago da una distancia temporal humana respecto a `now` (unix): "hace 3d".
func Ago(ts, now int64) string {
	if ts <= 0 {
		return "nunca"
	}
	secs := max(now-ts, 0)
	d := time.Duration(secs) * time.Second
	switch {
	case d < time.Minute:
		return "recién"
	case d < time.Hour:
		return fmt.Sprintf("hace %dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("hace %dh", int(d.Hours()))
	default:
		return fmt.Sprintf("hace %dd", int(d.Hours()/24))
	}
}
