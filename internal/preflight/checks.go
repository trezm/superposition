package preflight

import (
	"fmt"
	"os/exec"

	"github.com/peterje/superposition/internal/models"
)

func CheckAll() ([]models.CLIStatus, bool) {
	gitOk := checkGit()
	clis := []models.CLIStatus{
		checkCLI("claude"),
		checkCLI("codex"),
		checkCLI("gemini"),
	}

	if !gitOk {
		fmt.Println("⚠ git is not installed. Please install git to use Superposition.")
	}
	for _, cli := range clis {
		if !cli.Installed {
			fmt.Printf("⚠ %s is not installed. Install it to use %s sessions.\n", cli.Name, cli.Name)
		} else {
			fmt.Printf("✓ %s found (%s)\n", cli.Name, cli.Path)
		}
	}

	return clis, gitOk
}

func checkGit() bool {
	_, err := exec.LookPath("git")
	return err == nil
}

func checkCLI(name string) models.CLIStatus {
	path, err := exec.LookPath(name)
	if err != nil {
		return models.CLIStatus{Name: name, Installed: false}
	}
	// Auth is handled by the CLI itself inside the PTY session.
	// We only check that the binary exists on PATH.
	return models.CLIStatus{Name: name, Installed: true, Authed: true, Path: path}
}
