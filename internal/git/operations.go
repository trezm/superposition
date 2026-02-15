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

	// git clone --bare doesn't set a fetch refspec. Configure it to fetch into
	// a remote-tracking namespace so it won't conflict with worktree checkouts.
	exec.Command("git", "-C", localPath, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*").Run()

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
	// Fetch into a remote-tracking namespace to avoid conflicts with branches
	// checked out in worktrees. We then fast-forward local branches that
	// aren't currently checked out.
	exec.Command("git", "-C", barePath, "config", "remote.origin.fetch", "+refs/heads/*:refs/remotes/origin/*").Run()

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

	// Update local branches from remote-tracking refs, skipping any that are
	// checked out in a worktree.
	checkedOut := worktreeBranches(barePath)
	remotes, _ := exec.Command("git", "-C", barePath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin/").Output()
	for _, line := range strings.Split(strings.TrimSpace(string(remotes)), "\n") {
		branch := strings.TrimPrefix(line, "origin/")
		if branch == "" || branch == "HEAD" {
			continue
		}
		if checkedOut[branch] {
			continue
		}
		// Fast-forward the local branch to match the remote-tracking ref
		exec.Command("git", "-C", barePath, "update-ref", "refs/heads/"+branch, "refs/remotes/origin/"+branch).Run()
	}

	return nil
}

// worktreeBranches returns the set of branch names currently checked out in any worktree.
func worktreeBranches(barePath string) map[string]bool {
	out, err := exec.Command("git", "-C", barePath, "worktree", "list", "--porcelain").Output()
	if err != nil {
		return nil
	}
	branches := make(map[string]bool)
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "branch refs/heads/") {
			branches[strings.TrimPrefix(line, "branch refs/heads/")] = true
		}
	}
	return branches
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

	// Prefer the remote-tracking ref so we always base off the latest fetched
	// state, and fall back to the local branch ref.
	base := sourceBranch
	if err := exec.Command("git", "-C", barePath, "rev-parse", "--verify", "refs/remotes/origin/"+sourceBranch).Run(); err == nil {
		base = "refs/remotes/origin/" + sourceBranch
	}

	cmd := exec.Command("git", "-C", barePath, "worktree", "add", "-b", newBranch, worktreePath, base)
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
