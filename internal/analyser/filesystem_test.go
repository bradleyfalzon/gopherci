
	"github.com/bradleyfalzon/gopherci/internal/db"
	cfg := Config{
		BaseRepoURL: "base-url",
		BaseBranch:  "base-branch",
		HeadRepoURL: "head-url",
		HeadBranch:  "head-branch",
		DiffURL:     ts.URL,
	}

	tools := []db.Tool{
		{Name: "Name", Path: "tool", Args: "arg", ArgBaseSHA: "-flag"},
	}

			{"git", "clone", "--branch", cfg.HeadBranch, "--depth", "0", "--single-branch", cfg.HeadRepoURL, "/tmp/src/gopherci/rand"},
			{"git", "fetch", cfg.BaseRepoURL, cfg.BaseBranch},
			{"tool", "-flag", "FETCH_HEAD", "arg"},
		coOut: [][]byte{{}, {}, []byte(`main.go:1: error`)},
	issues, err := fs.Analyse(tools, cfg)
	expected := []Issue{{File: "main.go", HunkPos: 1, Issue: "Name: error"}}