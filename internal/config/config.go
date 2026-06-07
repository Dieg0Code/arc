package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Scope define un alcance de acceso de lectura: el conjunto de chats que un
// agente puede ver. Un chat entra en el scope si su título matchea alguno de
// Titles (globs) o su id está en Chats, y además su source está en Sources (si
// Sources está vacío, no filtra por source).
type Scope struct {
	Titles  []string `toml:"titles"`
	Sources []string `toml:"sources"`
	Chats   []string `toml:"chats"`
}

// file es el contenido de ~/.nem/config.toml.
type file struct {
	Scopes map[string]Scope `toml:"scopes"`
}

// Scopes carga los scopes definidos en ~/.nem/config.toml. Si el archivo no
// existe, devuelve un mapa vacío sin error: "sin scopes" es el default natural
// (acceso completo).
func Scopes() (map[string]Scope, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Scope{}, nil
		}
		return nil, fmt.Errorf("failed to read config %s: %w", path, err)
	}
	var f file
	if err := toml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("failed to parse config %s: %w", path, err)
	}
	if f.Scopes == nil {
		return map[string]Scope{}, nil
	}
	return f.Scopes, nil
}
