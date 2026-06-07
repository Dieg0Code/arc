package index

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Dieg0Code/nem/internal/db"
)

// HeuristicSummary arma un resumen barato (sin LLM) que describe la TAREA del
// chat: saltea boilerplate (AGENTS.md, /init, IDE-context, "session continued"),
// desenvuelve el pedido real, lo limpia a un lead de 1-2 frases, y agrega una
// cola corta de keywords salientes (mejor recall en búsqueda). Es el default de
// nem y también el fallback cuando el backend LLM falla.
func HeuristicSummary(chat db.Chat, firstMsgs []db.Message) string {
	lead := pickLead(firstMsgs)
	if lead == "" {
		if t := strings.TrimSpace(chat.Title); t != "" {
			return t
		}
		return "(empty chat)"
	}

	// Cola de topics (acotada). El lead manda; la cola ocupa lo que sobre.
	tail := ""
	if kws := keywords(firstMsgs, chat.Title, 5); len(kws) > 0 {
		tail = " · topics: " + strings.Join(kws, ", ")
		for len([]rune(tail)) > 80 && len(kws) > 1 { // recortar si quedó largo
			kws = kws[:len(kws)-1]
			tail = " · topics: " + strings.Join(kws, ", ")
		}
	}
	leadMax := maxSummaryChars - len([]rune(tail))
	return truncate(lead, leadMax) + tail
}

// pickLead elige el primer mensaje de usuario útil (no boilerplate), lo
// desenvuelve y lo limpia. Cae a assistant, luego al primer mensaje limpiable.
func pickLead(msgs []db.Message) string {
	// Preferimos el primer mensaje de usuario SUSTANCIAL (no un saludo suelto
	// como "hola"/"dale"); si solo hay cortos, usamos el primero igual.
	const substantial = 20
	firstShort := ""
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		body := unwrapRequest(m.Content)
		if strings.TrimSpace(body) == "" || isBoilerplate(body) {
			continue
		}
		lead := cleanLead(body)
		if lead == "" {
			continue
		}
		if len([]rune(lead)) >= substantial {
			return lead
		}
		if firstShort == "" {
			firstShort = lead
		}
	}
	if firstShort != "" {
		return firstShort
	}
	// Sin pedido de usuario claro: probar el primer assistant no-boilerplate.
	for _, m := range msgs {
		if m.Role != "assistant" || isBoilerplate(m.Content) {
			continue
		}
		if lead := cleanLead(m.Content); lead != "" {
			return lead
		}
	}
	return ""
}

// boilerplatePrefixes: inicios (lowercase) que delatan ruido de sistema/setup en
// vez del pedido real. Derivados de la data real ingestada.
var boilerplatePrefixes = []string{
	"# agents.md", "agents.md instructions", "<instructions",
	"repository guidelines", "# claude.md", "claude.md",
	"this session is being continued",
	"# context from my ide setup",
	"here is a summary of the conversation",
	"caveat:",
}

// boilerplateExact: mensajes que son SOLO un comando/slash sin contenido real.
var boilerplateExact = map[string]struct{}{
	"init": {}, "/init": {}, "/resume": {}, "/compact": {}, "/clear": {},
	"continue": {}, "go on": {}, "sigue": {}, "continúa": {}, "continua": {},
}

// isBoilerplate decide si un mensaje es ruido de setup/sistema y no la tarea.
func isBoilerplate(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	if t == "" {
		return true
	}
	if _, ok := boilerplateExact[t]; ok {
		return true
	}
	for _, p := range boilerplatePrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	// Wrapper de IDE-context de Codex (sin el "## My request" ya desenvuelto).
	if strings.Contains(t, "## active file") && strings.Contains(t, "## open tabs") {
		return true
	}
	// Mensaje que es esencialmente un solo bloque de código / volcado de archivo.
	if strings.HasPrefix(t, "```") {
		return true
	}
	return false
}

// reWrappers: bloques XML de sistema/setup que envuelven al pedido real (Codex y
// Claude). Se remueven para quedarnos con el contenido humano. RE2 no soporta
// backreferences, así que van uno por uno.
var reWrappers = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<system-reminder>.*?</system-reminder>`),
	regexp.MustCompile(`(?is)<instructions>.*?</instructions>`),
	regexp.MustCompile(`(?is)<environment_context>.*?</environment_context>`),
	regexp.MustCompile(`(?is)<user_instructions>.*?</user_instructions>`),
	// Wrappers de slash-commands de Claude Code (/init, /resume, /model, …).
	regexp.MustCompile(`(?is)<command-message>.*?</command-message>`),
	regexp.MustCompile(`(?is)<command-name>.*?</command-name>`),
	regexp.MustCompile(`(?is)<command-args>.*?</command-args>`),
	regexp.MustCompile(`(?is)<local-command-caveat>.*?</local-command-caveat>`),
	regexp.MustCompile(`(?is)<bash-input>.*?</bash-input>`),
	regexp.MustCompile(`(?is)<bash-stdout>.*?</bash-stdout>`),
	regexp.MustCompile(`(?is)<bash-stderr>.*?</bash-stderr>`),
	regexp.MustCompile(`(?is)<local-command-stdout>.*?</local-command-stdout>`),
	regexp.MustCompile(`(?is)<local-command-stderr>.*?</local-command-stderr>`),
	// Bloques de tool-call/result de Codex (contenido entre marcadores).
	regexp.MustCompile(`(?is)\[external_agent_tool_call.*?\[/external_agent_tool_call\]`),
	regexp.MustCompile(`(?is)\[external_agent_tool_result.*?\[/external_agent_tool_result\]`),
}

var (
	// reArtifacts: marcadores de tool calls/resultados e imágenes que mete el
	// ingest, ruido para un resumen.
	reArtifacts = regexp.MustCompile(`(?is)\[/?external[^\]]*\]|\[image[^\]]*\]|\[request interrupted[^\]]*\]`)
	reCodeFence = regexp.MustCompile("(?s)```.*?```")
	reMdLink    = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
	reMdLead    = regexp.MustCompile(`(?m)^[#>\s*\-]+`)
	reWord      = regexp.MustCompile(`[\p{L}][\p{L}\p{N}_+-]{2,}`)
	reSentence  = regexp.MustCompile(`[.?!](\s|$)`)
)

// stripWrappers remueve bloques de sistema/setup (XML de Codex/Claude, slash-
// commands, tool-calls) y marcadores de tool/imagen. Idempotente.
func stripWrappers(s string) string {
	for _, re := range reWrappers {
		s = re.ReplaceAllString(s, " ")
	}
	return reArtifacts.ReplaceAllString(s, " ")
}

// unwrapRequest extrae el pedido real embebido en un mensaje con wrappers.
func unwrapRequest(s string) string {
	// El wrapper de Codex pone el pedido tras este encabezado.
	for _, marker := range []string{"## My request for Codex:", "My request for Codex:"} {
		if i := strings.Index(s, marker); i >= 0 {
			s = s[i+len(marker):]
			break
		}
	}
	return strings.TrimSpace(stripWrappers(s))
}

// cleanLead limpia wrappers/markdown/código y devuelve las primeras 1-2 frases.
func cleanLead(s string) string {
	s = stripWrappers(s)                     // wrappers de sistema/tool
	s = reCodeFence.ReplaceAllString(s, " ") // bloques de código
	s = reMdLink.ReplaceAllString(s, "$1")   // [texto](url) -> texto
	s = strings.ReplaceAll(s, "`", "")       // inline code
	s = strings.ReplaceAll(s, "*", "")       // bold/italic
	s = reMdLead.ReplaceAllString(s, "")     // #, >, -, * líderes por línea
	s = oneLine(s)                           // colapsar espacios
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return firstSentences(s, 220)
}

// firstSentences toma 1-2 oraciones hasta `max` runas, cortando en límite de
// oración si se puede; si no, trunca por longitud.
func firstSentences(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	// Buscar el último fin de oración dentro del presupuesto.
	window := string(r[:max])
	locs := reSentence.FindAllStringIndex(window, -1)
	if len(locs) > 0 {
		end := locs[len(locs)-1][0] + 1 // incluir el signo de puntuación
		if end >= 40 {                  // evitar cortes demasiado cortos
			return strings.TrimSpace(window[:end])
		}
	}
	return truncate(s, max)
}

// keywords extrae los top-n términos salientes (frecuencia, sin stopwords),
// dedup contra las palabras del título.
func keywords(msgs []db.Message, title string, n int) []string {
	titleWords := map[string]struct{}{}
	for _, w := range reWord.FindAllString(strings.ToLower(title), -1) {
		titleWords[w] = struct{}{}
	}

	type kw struct {
		word  string
		count int
		first int // orden de primera aparición (desempate estable)
	}
	seen := map[string]*kw{}
	var order []*kw
	idx := 0
	for _, m := range msgs {
		if m.Role != "user" && m.Role != "assistant" {
			continue
		}
		body := unwrapRequest(m.Content)
		if isBoilerplate(body) {
			continue
		}
		body = reCodeFence.ReplaceAllString(body, " ")
		for _, w := range reWord.FindAllString(strings.ToLower(body), -1) {
			if _, stop := stopwords[w]; stop {
				continue
			}
			if _, inTitle := titleWords[w]; inTitle {
				continue
			}
			if e, ok := seen[w]; ok {
				e.count++
			} else {
				e := &kw{word: w, count: 1, first: idx}
				seen[w] = e
				order = append(order, e)
				idx++
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		if order[i].count != order[j].count {
			return order[i].count > order[j].count
		}
		return order[i].first < order[j].first
	})
	out := make([]string, 0, n)
	for _, e := range order {
		out = append(out, e.word)
		if len(out) == n {
			break
		}
	}
	return out
}

// stopwords: relleno gramatical ES+EN que no aporta como "topic". No incluye
// términos de dominio (kotlin, ataxx, etc.).
var stopwords = map[string]struct{}{}

func init() {
	list := strings.Fields(`
		the and for that this with you are not but have has had was were what how when
		which then them they she his her its our your their can will just like want need
		make get let use using from into out about over under more most some any all each
		one two also very much many such only than too here there been being does did done
		should would could may might must shall into onto upon per via etc yes okay
		el la los las un una unos unas de del que con sin por para como pero más mas muy
		este esta esto estos estas ese esa eso esos esas aqui aquí ahi ahí cuando donde
		porque pues son fue era ser estar hay hace hacer todo toda todos todas algo nada
		ya no si sí lo le les nos su sus mi mis tu tus me te se y o u a en es al
		oye dale bueno entonces igual quiero quieres puede puedes vamos vos
		qué cómo cuál cuáles cuándo dónde voy vas van mira porfa tenemos tengo
		tienes gustaria gustaría haciendo primero hacer hace hola este esta
	`)
	for _, w := range list {
		stopwords[w] = struct{}{}
	}
}
