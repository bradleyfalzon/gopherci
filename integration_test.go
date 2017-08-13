//+build integration_github

package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/github"
	"github.com/joho/godotenv"
	"golang.org/x/oauth2"
)

// IntegrationTest helps run a single integration test. Focusing on interaction
// between GopherCI and GitHub, IntegrationTest helps write tests that ensures
// GopherCI receives hooks, detects issues and posts comments.
type IntegrationTest struct {
	t             *testing.T
	workdir       string
	tmpdir        string
	owner         string
	repo          string
	github        *github.Client
	gciCancelFunc context.CancelFunc
	env           []string
}

// NewIntegrationTest creates an environment for running integration tests by
// creating file system temporary directories, starting gopherci and setting
// up a github repository. Must be closed when finished.
func NewIntegrationTest(ctx context.Context, t *testing.T) *IntegrationTest {
	// Load environment from .env, ignore errors as it's optional and dev only
	_ = godotenv.Load()

	workdir, err := os.Getwd()
	if err != nil {
		t.Fatalf("could not get working dir: %s", err)
	}

	// Make a temp dir which will be our github repository.
	tmpdir, err := ioutil.TempDir("", "gopherci-integration")
	if err != nil {
		t.Fatalf("could not create temporary directory: %v", err)
	}

	it := &IntegrationTest{t: t, workdir: workdir, tmpdir: tmpdir, owner: "bf-test", repo: "gopherci-itest"} // TODO config
	it.t.Logf("GitHub owner %q repo %q tmpdir %q workdir %q", it.owner, it.repo, it.tmpdir, it.workdir)

	// Force git to use our SSH key, requires Git 2.3+.
	if os.Getenv("INTEGRATION_GITHUB_KEY_FILE") != "" {
		it.env = append(it.env, fmt.Sprintf("GIT_SSH_COMMAND=ssh -i %v", os.Getenv("INTEGRATION_GITHUB_KEY_FILE")))
	}

	// Look for binaries (just git currently) in an alternative path.
	if os.Getenv("INTEGRATION_PATH") != "" {
		it.env = append(it.env, "PATH="+os.Getenv("INTEGRATION_PATH"))
	}
	it.t.Logf("Additional environment for os/exec.Command (may be empty): %v", it.env)

	// Setup GitHub Client.
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: os.Getenv("INTEGRATION_GITHUB_PAT")},
	)
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	it.github = github.NewClient(tc)
	it.t.Logf("GitHub Personal Access Token len %v", len(os.Getenv("INTEGRATION_GITHUB_PAT")))

	// Obtain the clone URL and also test the personal access token.
	repo, _, err := it.github.Repositories.Get(ctx, it.owner, it.repo)
	if err != nil {
		it.t.Fatalf("could not get repository information for %v/%v using personal access token: %v", it.owner, it.repo, err)
	}

	// Initialise the repository to known good state.
	it.Exec("init.sh", *repo.SSHURL)

	// Start GopherCI.
	it.gciCancelFunc = it.startGopherCI()

	return it
}

// startGopherCI runs gopherci in the background and returns a function to be
// called when it should be terminated. Writes output to test log functions
// so they should only appear if the test fails.
func (it *IntegrationTest) startGopherCI() context.CancelFunc {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		out, err := exec.CommandContext(ctx, "gopherci").CombinedOutput()
		if err != nil && err.Error() != "signal: killed" {
			// gopherci should always return without an error, if it doesn't
			// write all the output to help debug.
			it.t.Logf("Gopherci output:\n%s", out)
			it.t.Logf("Gopherci error: %v", err)
		}
	}()
	time.Sleep(10 * time.Second) // Wait for gopherci to listen on interface.
	it.t.Log("Started gopherci (this is not rebuilt, ensure you rebuild/install before running to test the latest changes)")
	return cancel
}

// Close stops gopherci and removes any temporary directories.
func (it *IntegrationTest) Close() {
	it.gciCancelFunc() // Kill gopherci.

	// We sleep a moment here to give the goroutine that was running gopherci
	// a chance to write its output to the tests's log function before the
	// entire test is terminated.
	time.Sleep(time.Second)

	if err := os.RemoveAll(it.tmpdir); err != nil {
		log.Printf("integration test close: could not remove %v: %v", it.tmpdir, err)
	}
}

// Exec executes a script within the testdata directory with args.
func (it *IntegrationTest) Exec(script string, args ...string) {
	cmd := exec.Command(filepath.Join(it.workdir, "testdata", script), args...)
	cmd.Env = it.env
	cmd.Dir = it.tmpdir
	out, err := cmd.CombinedOutput()
	if err != nil {
		it.t.Fatalf("could not run %v: %v, output:\n%s", cmd.Args, err, out)
	}
	it.t.Logf("executed %v", cmd.Args)
}

// WaitForSuccess waits for the statusContext's ref Status API to be success,
// will only wait for a short timeout, unless the status is failure or error,
// in which case the test is marked as failed. Returns the repo status (test
// aborts on failure).
func (it *IntegrationTest) WaitForSuccess(ref, statusContext string) *github.RepoStatus {
	timeout := 60 * time.Second
	start := time.Now()
	startedPending := false // first status must be pending
	for time.Now().Before(start.Add(timeout)) {
		statuses, _, err := it.github.Repositories.GetCombinedStatus(context.Background(), it.owner, it.repo, ref, nil)
		if err != nil {
			it.t.Fatalf("could not get combined statuses: %v", err)
		}

		for _, status := range statuses.Statuses {
			if *status.Context != statusContext {
				continue
			}
			it.t.Logf("Checking status: %q for ref %q", *status.State, ref)

			switch {
			case !startedPending:
				// Make sure the first status we see is a pending status.
				if want := "pending"; *status.State != want {
					it.t.Fatalf("first status for ref %v was %v want %v", ref, *status.State, want)
				}
				startedPending = true
			case *status.State == "success":
				return &status
			case *status.State == "failure" || *status.State == "error":
				it.t.Fatalf("status %v for ref %v", *status.State, ref)
			}
		}
		time.Sleep(2 * time.Second)
	}
	it.t.Fatalf("timeout waiting for status api to be success, failure or error")
	return nil
}

// TestGitHubComments_pushMultiple tests pushing multiple commits which should
// create a single summary commit comment.
func TestGitHubComments_pushMultiple(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	// Push a branch which contains a single issue in a single commit.
	branch := "issue-comments-multiple"
	it.Exec("issue-comments-multiple.sh", branch)

	it.WaitForSuccess(branch, "ci/gopherci/push")

	time.Sleep(5 * time.Second) // wait for comments to appear

	// Make sure the expected comments appear

	commits, _, err := it.github.Repositories.ListCommits(ctx, it.owner, it.repo, &github.CommitsListOptions{
		SHA: branch,
	})
	if err != nil {
		t.Fatalf("could not get commits: %v", err)
	}
	if len(commits) == 0 {
		t.Fatalf("list commits was empty on branch %v", branch)
	}

	sha := commits[0].GetSHA()
	comments, _, err := it.github.Repositories.ListCommitComments(ctx, it.owner, it.repo, sha, nil)
	if err != nil {
		t.Fatalf("could not get commits for sha %v on branch %v: %v", err, sha, branch)
	}

	if want := 1; len(comments) != want {
		t.Fatalf("have %v comments want %v on sha %v", len(comments), want, sha)
	}
	if comments[0].Position != nil {
		t.Fatalf("have comments position %v want %v", *comments[0].Position, nil)
	}
	if comments[0].Path != nil {
		t.Fatalf("have comments path %q want %q", *comments[0].Path, nil)
	}
	if prefix := "GopherCI found **2** issues in the last **2** commits, see: https://"; !strings.HasPrefix(*comments[0].Body, prefix) {
		t.Fatalf("comment body does not match expected prefix:\nhave: %q\nwant: %q", *comments[0].Body, prefix)
	}
}

// TestGitHubComments_pushSingle tests pushing a single commit which should
// create an inline commit comment.
func TestGitHubComments_pushSingle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	// Push a branch which contains a single issue in a single commit.
	branch := "issue-comments"
	it.Exec("issue-comments.sh", branch)

	it.WaitForSuccess(branch, "ci/gopherci/push")

	time.Sleep(5 * time.Second) // wait for comments to appear

	// Make sure the expected comments appear

	commits, _, err := it.github.Repositories.ListCommits(ctx, it.owner, it.repo, &github.CommitsListOptions{
		SHA: branch,
	})
	if err != nil {
		t.Fatalf("could not get commits: %v", err)
	}
	if len(commits) == 0 {
		t.Fatalf("list commits was empty on branch %v", branch)
	}

	sha := commits[0].GetSHA()
	comments, _, err := it.github.Repositories.ListCommitComments(ctx, it.owner, it.repo, sha, nil)
	if err != nil {
		t.Fatalf("could not get commits for sha %v on branch %v: %v", err, sha, branch)
	}

	if want := 1; len(comments) != want {
		t.Fatalf("have %v comments want %v on sha %v", len(comments), want, sha)
	}
	if want := 2; *comments[0].Position != want {
		t.Fatalf("have comments position %v want %v", *comments[0].Position, want)
	}
	if want := "foo.go"; *comments[0].Path != want {
		t.Fatalf("have comments path %q want %q", *comments[0].Path, want)
	}
	if want := "golint: exported function Foo should have comment or be unexported"; *comments[0].Body != want {
		t.Fatalf("unexpected comment body:\nhave: %q\nwant: %q", *comments[0].Body, want)
	}
}

func TestGitHubComments_pr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	// Push a branch which contains issues
	branch := "issue-comments"
	it.Exec("issue-comments.sh", branch)

	// Make PR
	pr, _, err := it.github.PullRequests.Create(ctx, it.owner, it.repo, &github.NewPullRequest{
		Title: github.String("pr title"),
		Head:  github.String(branch),
		Base:  github.String("master"),
	})
	if err != nil {
		t.Fatalf("could not create pull request: %v", err)
	}

	it.WaitForSuccess(branch, "ci/gopherci/pr")

	time.Sleep(5 * time.Second) // wait for comments to appear

	// Make sure review was completed
	reviews, _, err := it.github.PullRequests.ListReviews(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request reviews: %v", err)
	}

	if want := 1; len(reviews) != want {
		t.Fatalf("have %v reviews want %v", len(reviews), want)
	}

	if want := "COMMENTED"; reviews[0].GetState() != want {
		t.Fatalf("have %v review state want %v", reviews[0].GetState(), want)
	}

	// Make sure the expected comments appear
	comments, _, err := it.github.PullRequests.ListComments(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request comments: %v", err)
	}

	if want := 1; len(comments) != want {
		t.Fatalf("have %v comments want %v", len(comments), want)
	}
	if want := 2; *comments[0].Position != want {
		t.Fatalf("have comments position %v want %v", *comments[0].Position, want)
	}
	if want := "foo.go"; *comments[0].Path != want {
		t.Fatalf("have comments path %q want %q", *comments[0].Path, want)
	}
	if want := "golint: exported function Foo should have comment or be unexported"; *comments[0].Body != want {
		t.Fatalf("unexpected comment body:\nhave: %q\nwant: %q", *comments[0].Body, want)
	}

	it.Exec("dupe-issue-comments.sh")

	it.WaitForSuccess(branch, "ci/gopherci/pr")

	time.Sleep(5 * time.Second) // wait for comments to appear

	// dupe-issue-comments.sh pushes another issue to the same branch, there
	// will then be a scenario where gopherci's second run would duplicate the
	// existing comment.

	comments, _, err = it.github.PullRequests.ListComments(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request comments 2: %v", err)
	}

	if want := 2; len(comments) != want {
		t.Fatalf("have %v comments after second push want %v", len(comments), want)
	}
}

func TestGitHubComments_ignoreGenerated(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	// Push a branch which contains issues
	branch := "issue-comments-ignore-generated"
	it.Exec("issue-comments-ignore-generated.sh", branch)

	// Make PR
	pr, _, err := it.github.PullRequests.Create(ctx, it.owner, it.repo, &github.NewPullRequest{
		Title: github.String("pr title"),
		Head:  github.String(branch),
		Base:  github.String("master"),
	})
	if err != nil {
		t.Fatalf("could not create pull request: %v", err)
	}

	it.WaitForSuccess(branch, "ci/gopherci/pr")

	time.Sleep(5 * time.Second) // wait for comments to appear

	// Make sure the expected comments appear
	comments, _, err := it.github.PullRequests.ListComments(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request comments: %v", err)
	}

	if want := 0; len(comments) != want {
		t.Fatalf("have %v comments want %v", len(comments), want)
	}
}

func TestGitHubComments_none(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	// Push a branch which contains no issues
	branch := "issue-no-comments"
	it.Exec("issue-no-comments.sh", branch)

	// Make PR
	pr, _, err := it.github.PullRequests.Create(ctx, it.owner, it.repo, &github.NewPullRequest{
		Title: github.String("pr title"),
		Head:  github.String(branch),
		Base:  github.String("master"),
	})
	if err != nil {
		t.Fatalf("could not create pull request: %v", err)
	}

	it.WaitForSuccess(branch, "ci/gopherci/pr")

	time.Sleep(5 * time.Second) // wait for comments to appear (hopefully none)

	// Make sure review was completed
	reviews, _, err := it.github.PullRequests.ListReviews(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request reviews: %v", err)
	}

	if want := 1; len(reviews) != want {
		t.Fatalf("have %v reviews want %v", len(reviews), want)
	}

	if want := "APPROVED"; reviews[0].GetState() != want {
		t.Fatalf("have %v review state want %v", reviews[0].GetState(), want)
	}

	// Make sure no comments appear
	comments, _, err := it.github.PullRequests.ListComments(ctx, it.owner, it.repo, *pr.Number, nil)
	if err != nil {
		t.Fatalf("could not get pull request comments: %v", err)
	}

	if want := 0; len(comments) != want {
		t.Fatalf("have %v comments want %v", len(comments), want)
	}
}

// TestAnalysisView tests the HTML generated when viewing an analysis.
func TestAnalysisView(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	branch := "analysis-view"
	preexisting, _, err := it.github.Git.GetRef(ctx, it.owner, it.repo, "heads/"+branch)
	if err != nil && err.(*github.ErrorResponse).Response.StatusCode != http.StatusNotFound {
		t.Fatalf("could not get branch %v: %v", branch, err)
	}
	if preexisting != nil {
		_, err = it.github.Git.DeleteRef(ctx, it.owner, it.repo, "heads/"+branch)
		if err != nil {
			t.Fatalf("could not delete branch %v: %v", branch, err)
		}
	}

	want200OK := func(t *testing.T, targetURL string) {
		resp, err := http.Get(targetURL)
		if err != nil {
			t.Logf("test")
			t.Fatalf("unexpected error checking status target url: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status target url %v code: %v, want: %v", targetURL, resp.StatusCode, http.StatusOK)
		}
	}

	tests := []struct {
		arg string
	}{
		// An initial push
		{"step-1"},
		// Force push commit
		{"step-2"},
		// Push another commit as normal
		{"step-3"},
	}

	for _, test := range tests {
		it.Exec("analysis-view.sh", branch, test.arg)

		status := it.WaitForSuccess(branch, "ci/gopherci/push")
		want200OK(t, *status.TargetURL)
	}

	// Make a PR and check again

	_, _, err = it.github.PullRequests.Create(ctx, it.owner, it.repo, &github.NewPullRequest{
		Title: github.String("pr title - analysis view"),
		Head:  github.String(branch),
		Base:  github.String("master"),
	})
	if err != nil {
		t.Fatalf("could not create pull request: %v", err)
	}

	status := it.WaitForSuccess(branch, "ci/gopherci/pr")

	want200OK(t, *status.TargetURL)
}

// TestMissingBeforeBranch tests the case where the before ref does not exist,
// such as branch tracking different tree.
func TestMissingBeforeRef(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	it.Exec("new-tree.sh", "master")

	it.WaitForSuccess("master", "ci/gopherci/push")
}

// TestAPTPackagesCGO tests that cgo dependencies are installed via apt-get.
func TestAPTPackagesCGO(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	it := NewIntegrationTest(ctx, t)
	defer it.Close()

	it.Exec("apt-cgo.sh", "aptcgo")

	it.WaitForSuccess("aptcgo", "ci/gopherci/push")
}
