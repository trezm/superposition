package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// DiffResult holds the parsed output of a git diff.
type DiffResult struct {
	Files []DiffFile `json:"files"`
	Stats DiffStats  `json:"stats"`
}

// DiffStats holds aggregate statistics for a diff.
type DiffStats struct {
	FilesChanged int `json:"files_changed"`
	Additions    int `json:"additions"`
	Deletions    int `json:"deletions"`
}

// DiffFile represents a single file's diff.
type DiffFile struct {
	Path      string     `json:"path"`
	OldPath   string     `json:"old_path,omitempty"`
	Status    string     `json:"status"` // "added", "modified", "deleted", "renamed"
	Binary    bool       `json:"binary"`
	Hunks     []DiffHunk `json:"hunks"`
	Additions int        `json:"additions"`
	Deletions int        `json:"deletions"`
}

// DiffHunk represents a single hunk in a file diff.
type DiffHunk struct {
	Header   string     `json:"header"`
	OldStart int        `json:"old_start"`
	OldCount int        `json:"old_count"`
	NewStart int        `json:"new_start"`
	NewCount int        `json:"new_count"`
	Lines    []DiffLine `json:"lines"`
}

// DiffLine represents a single line in a diff hunk.
type DiffLine struct {
	Type    string `json:"type"` // "add", "delete", "context"
	Content string `json:"content"`
	OldNum  int    `json:"old_num,omitempty"`
	NewNum  int    `json:"new_num,omitempty"`
}

// ResolveCommit resolves a git ref to a full commit SHA.
func ResolveCommit(repoOrWorktreePath, ref string) (string, error) {
	cmd := exec.Command("git", "-C", repoOrWorktreePath, "rev-parse", ref)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s: %w", ref, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Diff computes the diff between a base commit and the current working tree state.
func Diff(worktreePath, baseCommit string) (*DiffResult, error) {
	cmd := exec.Command("git", "-C", worktreePath, "diff", baseCommit)
	out, err := cmd.Output()
	if err != nil {
		// git diff returns exit code 1 when there are differences in some modes,
		// but with default mode it returns 0. Check for actual errors.
		if exitErr, ok := err.(*exec.ExitError); ok {
			if len(exitErr.Stderr) > 0 {
				return nil, fmt.Errorf("git diff: %s", string(exitErr.Stderr))
			}
		}
		return nil, fmt.Errorf("git diff: %w", err)
	}
	return parseDiff(string(out))
}

// parseDiff parses unified diff output into structured types.
func parseDiff(raw string) (*DiffResult, error) {
	result := &DiffResult{}

	if strings.TrimSpace(raw) == "" {
		return result, nil
	}

	lines := strings.Split(raw, "\n")
	var currentFile *DiffFile
	var currentHunk *DiffHunk
	oldNum, newNum := 0, 0

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// New file header
		if strings.HasPrefix(line, "diff --git ") {
			if currentHunk != nil && currentFile != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
				currentHunk = nil
			}
			if currentFile != nil {
				result.Files = append(result.Files, *currentFile)
			}
			currentFile = &DiffFile{Status: "modified"}
			currentHunk = nil
			continue
		}

		if currentFile == nil {
			continue
		}

		// Parse file metadata
		if strings.HasPrefix(line, "--- ") {
			path := strings.TrimPrefix(line, "--- ")
			if path == "/dev/null" {
				currentFile.Status = "added"
			} else {
				currentFile.OldPath = strings.TrimPrefix(path, "a/")
			}
			continue
		}
		if strings.HasPrefix(line, "+++ ") {
			path := strings.TrimPrefix(line, "+++ ")
			if path == "/dev/null" {
				currentFile.Status = "deleted"
			} else {
				currentFile.Path = strings.TrimPrefix(path, "b/")
			}
			continue
		}

		// Rename detection
		if strings.HasPrefix(line, "rename from ") {
			currentFile.OldPath = strings.TrimPrefix(line, "rename from ")
			currentFile.Status = "renamed"
			continue
		}
		if strings.HasPrefix(line, "rename to ") {
			currentFile.Path = strings.TrimPrefix(line, "rename to ")
			continue
		}

		// New file mode
		if strings.HasPrefix(line, "new file mode") {
			currentFile.Status = "added"
			continue
		}
		if strings.HasPrefix(line, "deleted file mode") {
			currentFile.Status = "deleted"
			continue
		}

		// Binary file
		if strings.HasPrefix(line, "Binary files") {
			currentFile.Binary = true
			// Try to extract path from "Binary files /dev/null and b/path differ"
			// or "Binary files a/path and b/path differ"
			parts := strings.Split(line, " and ")
			if len(parts) == 2 {
				bPath := strings.TrimSuffix(parts[1], " differ")
				bPath = strings.TrimPrefix(bPath, "b/")
				if currentFile.Path == "" {
					currentFile.Path = bPath
				}
			}
			continue
		}

		// Skip other metadata lines (index, similarity, etc.)
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "old mode") ||
			strings.HasPrefix(line, "new mode") || strings.HasPrefix(line, "similarity") ||
			strings.HasPrefix(line, "dissimilarity") || strings.HasPrefix(line, "copy from") ||
			strings.HasPrefix(line, "copy to") {
			continue
		}

		// Hunk header
		if strings.HasPrefix(line, "@@") {
			if currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}
			hunk, err := parseHunkHeader(line)
			if err != nil {
				continue
			}
			currentHunk = hunk
			oldNum = hunk.OldStart
			newNum = hunk.NewStart
			continue
		}

		if currentHunk == nil {
			continue
		}

		// No newline at end of file marker
		if line == "\\ No newline at end of file" {
			continue
		}

		// Diff content lines
		if strings.HasPrefix(line, "+") {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "add",
				Content: strings.TrimPrefix(line, "+"),
				NewNum:  newNum,
			})
			newNum++
			currentFile.Additions++
			result.Stats.Additions++
		} else if strings.HasPrefix(line, "-") {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "delete",
				Content: strings.TrimPrefix(line, "-"),
				OldNum:  oldNum,
			})
			oldNum++
			currentFile.Deletions++
			result.Stats.Deletions++
		} else if strings.HasPrefix(line, " ") {
			currentHunk.Lines = append(currentHunk.Lines, DiffLine{
				Type:    "context",
				Content: strings.TrimPrefix(line, " "),
				OldNum:  oldNum,
				NewNum:  newNum,
			})
			oldNum++
			newNum++
		}
	}

	// Flush last hunk and file
	if currentHunk != nil && currentFile != nil {
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
	}
	if currentFile != nil {
		result.Files = append(result.Files, *currentFile)
	}

	result.Stats.FilesChanged = len(result.Files)
	return result, nil
}

// parseHunkHeader parses a hunk header like "@@ -1,5 +1,7 @@ optional context".
func parseHunkHeader(line string) (*DiffHunk, error) {
	// Find the range info between @@ markers
	if !strings.HasPrefix(line, "@@") {
		return nil, fmt.Errorf("not a hunk header")
	}

	end := strings.Index(line[2:], "@@")
	if end == -1 {
		return nil, fmt.Errorf("malformed hunk header")
	}

	rangeInfo := strings.TrimSpace(line[2 : end+2])
	parts := strings.Split(rangeInfo, " ")
	if len(parts) < 2 {
		return nil, fmt.Errorf("malformed hunk range")
	}

	hunk := &DiffHunk{Header: line}

	// Parse old range (-start,count)
	oldRange := strings.TrimPrefix(parts[0], "-")
	oldParts := strings.Split(oldRange, ",")
	hunk.OldStart, _ = strconv.Atoi(oldParts[0])
	if len(oldParts) > 1 {
		hunk.OldCount, _ = strconv.Atoi(oldParts[1])
	} else {
		hunk.OldCount = 1
	}

	// Parse new range (+start,count)
	newRange := strings.TrimPrefix(parts[1], "+")
	newParts := strings.Split(newRange, ",")
	hunk.NewStart, _ = strconv.Atoi(newParts[0])
	if len(newParts) > 1 {
		hunk.NewCount, _ = strconv.Atoi(newParts[1])
	} else {
		hunk.NewCount = 1
	}

	return hunk, nil
}
