package install

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	b, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

func gitInit(dest string) error {
	// An empty --template keeps template hooks out of the new repository.
	if out, err := git(dest, "init", "-q", "--template=", "."); err != nil {
		return fmt.Errorf("falló `git init`: %v\n%s", err, out)
	}
	return nil
}

// commitInstall makes the root commit, ignoring any global hooks.
func commitInstall(dest, version string) error {
	if out, err := git(dest, "add", "-A"); err != nil {
		return fmt.Errorf("falló `git add`: %v\n%s", err, out)
	}
	hooks, err := os.MkdirTemp("", "aquas-nohooks-")
	if err != nil {
		return fmt.Errorf("no se pudo crear la carpeta de ganchos vacía: %v", err)
	}
	defer func() { _ = os.RemoveAll(hooks) }()
	subject := "Install Aquas Light v" + version
	out, err := git(dest, "-c", "core.hooksPath="+hooks, "-c", "commit.gpgsign=false",
		"commit", "-q", "-m", subject)
	if err != nil {
		name, _ := git(dest, "config", "user.name")
		email, _ := git(dest, "config", "user.email")
		if name == "" || email == "" {
			return fmt.Errorf("%w\n%s", errNoIdentity, out)
		}
		return fmt.Errorf("falló `git commit`: %v\n%s", err, out)
	}
	return nil
}
