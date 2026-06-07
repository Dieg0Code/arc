package skill

import _ "embed"

// skillTemplate es el contenido de SKILL.md compilado dentro del binario. Se
// edita como archivo real (internal/skill/SKILL.md) pero viaja embebido para
// que nem siga siendo un único ejecutable.
//
//go:embed SKILL.md
var skillTemplate string

// Template devuelve el contenido del skill que nem instala. Exportado para
// testing y para que otros comandos puedan leerlo.
func Template() string {
	return skillTemplate
}
