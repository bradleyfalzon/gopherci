package analyser

// Analyser analyses a repository and branch, returns issues found in patch
// or an error.
type Analyser interface {
	Analyse(repoURL, branch, patchURL string) error
}
