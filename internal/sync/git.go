package sync

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitRepo es un wrapper fino sobre `git -C <dir> ...`. El usuario nunca ve git
// directamente; arc lo maneja por debajo en sync/clone.
type gitRepo struct {
	dir string
}

// run ejecuta un comando git en el repo y devuelve stdout (trim). En error,
// incluye stderr para diagnóstico.
func (g gitRepo) run(args ...string) (string, error) {
	full := append([]string{"-C", g.dir}, args...)
	cmd := exec.Command("git", full...)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

// isRepo indica si dir ya es un repo git.
func (g gitRepo) isRepo() bool {
	_, err := os.Stat(filepath.Join(g.dir, ".git"))
	return err == nil
}

// initRepo inicializa el repo si no existe.
func (g gitRepo) initRepo() error {
	if g.isRepo() {
		return nil
	}
	if _, err := g.run("init"); err != nil {
		return err
	}
	return nil
}

// remoteAdd agrega (o reemplaza) un remote.
func (g gitRepo) remoteAdd(name, url string) error {
	if g.hasRemote(name) {
		_, err := g.run("remote", "set-url", name, url)
		return err
	}
	_, err := g.run("remote", "add", name, url)
	return err
}

// remotesVerbose devuelve la salida de `git remote -v`.
func (g gitRepo) remotesVerbose() (string, error) {
	return g.run("remote", "-v")
}

// hasRemote indica si existe un remote con ese nombre.
func (g gitRepo) hasRemote(name string) bool {
	out, err := g.run("remote")
	if err != nil {
		return false
	}
	for _, r := range strings.Fields(out) {
		if r == name {
			return true
		}
	}
	return false
}

// currentBranch devuelve la rama actual (default "main" si el repo está vacío).
func (g gitRepo) currentBranch() string {
	out, err := g.run("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil || out == "" || out == "HEAD" {
		return "main"
	}
	return out
}

// addAll stagea los paths dados dentro del repo.
func (g gitRepo) addAll(paths ...string) error {
	_, err := g.run(append([]string{"add"}, paths...)...)
	return err
}

// commit crea un commit. Devuelve false si no había nada que commitear.
func (g gitRepo) commit(message string) (bool, error) {
	// ¿Hay algo staged?
	if _, err := g.run("diff", "--cached", "--quiet"); err == nil {
		return false, nil // exit 0 = sin cambios staged
	}
	if _, err := g.run("commit", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// pullRebase trae cambios del remoto rebasando. Tolera el caso "sin upstream
// todavía" (primer push).
func (g gitRepo) pullRebase(remote, branch string) error {
	out, err := g.run("ls-remote", "--heads", remote, branch)
	if err != nil {
		return err
	}
	if strings.TrimSpace(out) == "" {
		return nil // la rama no existe en el remoto todavía: nada que traer
	}
	_, err = g.run("pull", "--rebase", remote, branch)
	return err
}

// push empuja la rama al remoto, fijando upstream.
func (g gitRepo) push(remote, branch string) error {
	_, err := g.run("push", "-u", remote, branch)
	return err
}
