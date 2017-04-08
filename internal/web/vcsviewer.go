package web

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"

	"sourcegraph.com/sourcegraph/go-diff/diff"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/github"
	"github.com/pkg/errors"
)

// A VCSReader reads information about a completed analysis.
type VCSReader interface {
	// Diff returns a multi file unified diff as a io.ReadCloser, or an error.
	Diff(ctx context.Context, repositoryID int, commitFrom string, commitTo string, requestNumber int) (io.ReadCloser, error)
}

// NewVCS returns a VCSReader for a given analysis.
func NewVCS(github *github.GitHub, analysis *db.Analysis) (VCSReader, error) {
	switch {
	case analysis.InstallationID != 0:
		// GitHub VCS
		return github.NewInstallation(analysis.InstallationID)
	default:
		// Unknown VCS
		return nil, errors.New("error determining VCS")
	}
}

// A Patch represents a single file's patch.
type Patch struct {
	Path  string
	Hunks []Hunk
}

// A Hunk represents a single hunk of changed lines.
type Hunk struct {
	Range string
	Lines []Line
}

// ChangeType is type of change that affects a line, such as added or removed.
type ChangeType string

const (
	// ChangeAdd means a line was added.
	ChangeAdd ChangeType = "add"
	// ChangeRemove means a line was removed.
	ChangeRemove ChangeType = "remove"
	// ChangeNone means a line was unchanged.
	ChangeNone ChangeType = "none"
)

// A Line represents a single line in a diff.
type Line struct {
	Line       string
	ChangeType ChangeType
	LineNo     int
	Issues     []db.Issue
}

// DiffIssues reads a diff and adds the issues to the lines affected. Only
// hunks with issues will be returned.
func DiffIssues(ctx context.Context, diffReader io.Reader, issues []db.Issue) ([]Patch, error) {
	ghDiff, err := ioutil.ReadAll(&io.LimitedReader{R: diffReader, N: 1e9})
	if err != nil {
		return nil, errors.Wrap(err, "could not read from diff reader")
	}

	fileDiffs, err := diff.ParseMultiFileDiff(ghDiff)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse diff")
	}

	var patches []Patch
	for _, fileDiff := range fileDiffs {
		file := Patch{
			Path: fileDiff.NewName[2:], // strip leading "a/" or "b/"
		}

		var fileHasIssues bool
		for _, fileHunk := range fileDiff.Hunks {
			scanner := bufio.NewScanner(bytes.NewReader(fileHunk.Body))

			hunk := Hunk{
				Range: fmt.Sprintf("@@ -%d,%d +%d,%d @@", fileHunk.OrigStartLine, fileHunk.OrigLines, fileHunk.NewStartLine, fileHunk.NewLines),
			}

			var hunkHasIssues bool
			for diffLineNo := int(fileHunk.NewStartLine); scanner.Scan(); diffLineNo++ {
				if len(scanner.Text()) == 0 {
					return nil, fmt.Errorf("file: %q, hunk: %q body contains empty line", file.Path, hunk.Range)
				}

				var changeType = ChangeNone
				switch scanner.Text()[0] {
				case byte('+'):
					changeType = ChangeAdd
				case byte('-'):
					changeType = ChangeRemove
				}

				// Find issues matching this line, ignore removed lines as an
				// issue may appear on the same line number that replaced this.
				var lineIssues []db.Issue
				if changeType != ChangeRemove {
					for _, issue := range issues {
						if issue.Path == file.Path && issue.Line == diffLineNo {
							hunkHasIssues = true
							lineIssues = append(lineIssues, issue)
						}
					}
				}

				hunk.Lines = append(hunk.Lines, Line{
					ChangeType: changeType,
					LineNo:     diffLineNo,
					Line:       scanner.Text()[1:],
					Issues:     lineIssues,
				})

				if changeType == ChangeRemove {
					diffLineNo--
				}
			}
			if scanner.Err() != nil {
				return nil, errors.Wrapf(err, "errors scanning file %v", file.Path)
			}

			if hunkHasIssues {
				fileHasIssues = true
				file.Hunks = append(file.Hunks, hunk)
			}
		}

		if fileHasIssues {
			patches = append(patches, file)
		}
	}
	return patches, nil
}
