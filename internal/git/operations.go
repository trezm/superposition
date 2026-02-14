package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/peterje/superposition/internal/db"
)

func ReposDir() (string, error) {
	dataDir, err := db.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "repos"), nil
}

func WorktreesDir() (string, error) {
	dataDir, err := db.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, "worktrees"), nil
}

// CloneBare clones a repo as a bare repository.
// For private repos, the PAT is embedded in the URL.
func CloneBare(cloneURL, pat, owner, name string) (string, error) {
	reposDir, err := ReposDir()
	if err != nil {
		return "", err
	}

	destDir := filepath.Join(reposDir, owner)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create repos dir: %w", err)
	}

	localPath := filepath.Join(destDir, name+".git")

	// If already exists, just fetch
	if _, err := os.Stat(localPath); err == nil {
		return localPath, Fetch(localPath, pat)
	}

	authURL := cloneURL
	if pat != "" {
		authURL = strings.Replace(cloneURL, "https://", "https://x-access-token:"+pat+"@", 1)
	}

	cmd := exec.Command("git", "clone", "--bare", authURL, localPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone: %s: %w", string(out), err)
	}

	// git clone --bare doesn't set a fetch refspec, so git fetch won't update
	// local branch refs. Configure it so fetch maps remote branches to local ones.
	exec.Command("git", "-C", localPath, "config", "remote.origin.fetch", "+refs/heads/*:refs/heads/*").Run()

	return localPath, nil
}

// CloneBareLocal clones a local git repo as a bare repository.
// No PAT needed — uses direct filesystem path as origin.
func CloneBareLocal(sourcePath, name string) (string, error) {
	// Validate that sourcePath is a git repo
	cmd := exec.Command("git", "-C", sourcePath, "rev-parse", "--git-dir")
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("not a git repository: %s: %w", strings.TrimSpace(string(out)), err)
	}

	reposDir, err := ReposDir()
	if err != nil {
		return "", err
	}

	destDir := filepath.Join(reposDir, "local")
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create repos dir: %w", err)
	}

	localPath := filepath.Join(destDir, name+".git")

	// If already exists, verify origin matches and fetch
	if _, err := os.Stat(localPath); err == nil {
		originCmd := exec.Command("git", "-C", localPath, "remote", "get-url", "origin")
		out, err := originCmd.Output()
		if err == nil {
			existingOrigin := strings.TrimSpace(string(out))
			if existingOrigin != sourcePath {
				return "", fmt.Errorf("bare repo already exists with different origin: %s", existingOrigin)
			}
		}
		return localPath, Fetch(localPath, "")
	}

	cloneCmd := exec.Command("git", "clone", "--bare", sourcePath, localPath)
	if out, err := cloneCmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git clone: %s: %w", string(out), err)
	}

	// Configure fetch refspec for bare clone
	exec.Command("git", "-C", localPath, "config", "remote.origin.fetch", "+refs/heads/*:refs/heads/*").Run()

	return localPath, nil
}

func Fetch(barePath, pat string) error {
	// Ensure fetch refspec is configured (bare clones don't set this by default)
	exec.Command("git", "-C", barePath, "config", "remote.origin.fetch", "+refs/heads/*:refs/heads/*").Run()

	cmd := exec.Command("git", "-C", barePath, "fetch", "--all", "--prune")
	if pat != "" {
		cmd.Env = append(os.Environ(),
			fmt.Sprintf("GIT_ASKPASS=echo"),
			fmt.Sprintf("GIT_TERMINAL_PROMPT=0"),
		)
		// Set the remote URL with auth for fetch
		setURL := exec.Command("git", "-C", barePath, "remote", "set-url", "origin",
			getAuthURL(barePath, pat))
		setURL.Run() // best effort
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch: %s: %w", string(out), err)
	}
	return nil
}

func getAuthURL(barePath, pat string) string {
	cmd := exec.Command("git", "-C", barePath, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	// Strip any existing credentials
	url = strings.Replace(url, "x-access-token:"+pat+"@", "", 1)
	// Re-add our token
	if pat != "" {
		url = strings.Replace(url, "https://", "https://x-access-token:"+pat+"@", 1)
	}
	return url
}

// AddWorktree creates a new worktree with a new branch based off a source branch.
// newBranch is the name of the branch to create, sourceBranch is the branch to base it on.
func AddWorktree(barePath, worktreePath, newBranch, sourceBranch string) error {
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return fmt.Errorf("create worktree parent: %w", err)
	}

	cmd := exec.Command("git", "-C", barePath, "worktree", "add", "-b", newBranch, worktreePath, sourceBranch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %s: %w", string(out), err)
	}
	return nil
}

func RemoveWorktree(barePath, worktreePath string) error {
	cmd := exec.Command("git", "-C", barePath, "worktree", "remove", "--force", worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s: %w", string(out), err)
	}
	return nil
}

func RemoveBranch(barePath, branch string) error {
	cmd := exec.Command("git", "-C", barePath, "branch", "-D", branch)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git branch delete: %s: %w", string(out), err)
	}
	return nil
}

func ListBranches(barePath string) ([]string, error) {
	cmd := exec.Command("git", "-C", barePath, "branch", "--format=%(refname:short)")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git branch: %w", err)
	}

	var branches []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			branches = append(branches, line)
		}
	}
	return branches, nil
}
