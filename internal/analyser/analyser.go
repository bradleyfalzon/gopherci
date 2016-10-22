package analyser

// Analyser analyses a repository and branch, returns issues found in patch
// or an error.
type Analyser interface {
	Analyse(repoURL, branch, diffURL string) ([]Issue, error)
}

// Issue contains file, position and string describing a single issue.
type Issue struct {
	// File is the relative path name of the file.
	File string
	// HunkPos is the position relative to the files first hunk.
	HunkPos int
	// Issue is the issue.
	Issue string
}
