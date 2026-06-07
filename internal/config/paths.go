// Package config resuelve las rutas del store local de nem (~/.nem) y la
// configuración persistida en config.toml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// Dir devuelve la ruta raíz del store local de nem (~/.nem).
func Dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home directory: %w", err)
	}
	return filepath.Join(home, ".nem"), nil
}

// DBPath devuelve la ruta de la base SQLite local (~/.nem/nem.db).
func DBPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "nem.db"), nil
}

// ConfigPath devuelve la ruta del archivo de configuración (~/.nem/config.toml).
func ConfigPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// StoreDir devuelve la ruta del directorio versionado por git (~/.nem/store).
func StoreDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "store"), nil
}

// ChatsDir devuelve la ruta donde se exportan los .jsonl por commit
// (~/.nem/store/chats).
func ChatsDir() (string, error) {
	store, err := StoreDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(store, "chats"), nil
}
