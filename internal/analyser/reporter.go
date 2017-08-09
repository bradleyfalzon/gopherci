package analyser

import (
	"context"

	"github.com/bradleyfalzon/gopherci/internal/db"
)

// A Reporter reports the issues.
type Reporter interface {
	Report(context.Context, []db.Issue) error
}

// MaxIssueComments is the maximum number of comments that will be written
// on a pull request by writeissues. a pr may have more comments written if
// writeissues is called multiple times, such is multiple syncronise events.
const MaxIssueComments = 10

// Suppress returns a maximum amount of issues, if any are suppressed the total
// number suppressed is also returned.
func Suppress(issues []db.Issue, max int) (int, []db.Issue) {
	if len(issues) > max {
		return len(issues) - max, issues[:max]
	}
	return 0, issues
}
