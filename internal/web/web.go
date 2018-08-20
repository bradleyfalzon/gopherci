package web

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/github"
	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/go-chi/chi"
)

// Web handles general web/html responses (not API hooks).
type Web struct {
	logger    logger.Logger
	db        db.DB
	gh        *github.GitHub
	templates *template.Template
}

// NewWeb returns a new Web instance, or an error.
func NewWeb(logger logger.Logger, db db.DB, gh *github.GitHub) (*Web, error) {
	// Initialise html templates
	templates, err := template.ParseGlob("internal/web/templates/*.tmpl")
	if err != nil {
		return nil, err
	}

	web := &Web{
		logger:    logger,
		db:        db,
		gh:        gh,
		templates: templates,
	}
	return web, nil
}

// NotFoundHandler displays a 404 not found error
func (web *Web) NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	web.errorHandler(w, r, http.StatusNotFound, fmt.Sprintf("%q not found", r.URL))
}

// errorHandler handles an error message, with an optional description
func (web *Web) errorHandler(w http.ResponseWriter, r *http.Request, code int, desc string) {
	page := struct {
		Title  string
		Code   string // eg 400
		Status string // eg Bad Request
		Desc   string // eg Missing key foo
	}{fmt.Sprintf("%d - %s", code, http.StatusText(code)), strconv.Itoa(code), http.StatusText(code), desc}

	if page.Desc == "" {
		page.Desc = http.StatusText(code)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	if err := web.templates.ExecuteTemplate(w, "error.tmpl", page); err != nil {
		web.logger.With("error", err).Error("cannot parse error template")
	}
}

// AnalysisHandler displays a single analysis.
func (web *Web) AnalysisHandler(w http.ResponseWriter, r *http.Request) {
	analysisID, err := strconv.ParseInt(chi.URLParam(r, "analysisID"), 10, 32)
	if err != nil {
		web.errorHandler(w, r, http.StatusBadRequest, "Invalid analysis ID")
		return
	}

	logger_ := web.logger.With("analysisID", analysisID)

	analysis, err := web.db.GetAnalysis(int(analysisID))
	if err != nil {
		logger_.With("error", err).Error("cannot get analysis")
		web.errorHandler(w, r, http.StatusInternalServerError, "Could not get analysis")
		return
	}

	if analysis == nil {
		web.NotFoundHandler(w, r)
		return
	}

	outputs, err := web.db.AnalysisOutputs(analysis.ID)
	if err != nil {
		logger_.With("error", err).Error("cannot get analysis output")
		web.errorHandler(w, r, http.StatusInternalServerError, "Could not get analysis output")
		return
	}

	vcs, err := NewVCS(web.gh, analysis)
	if err != nil {
		logger_.With("error", err).Error("cannot get analysis VCS")
		web.errorHandler(w, r, http.StatusInternalServerError, "Could not get VCS")
		return
	}

	// TODO there may be a scenario where a diff isn't return (after a forced
	// push?), if so, we should just give the template the issues to render.
	// If no errors, give template nil issues.

	var patches []Patch
	if len(analysis.Issues()) > 0 {
		diffReader, err := vcs.Diff(r.Context(), analysis.RepositoryID, analysis.CommitFrom, analysis.CommitTo, analysis.RequestNumber)
		switch {
		case err != nil:
			// There is one remaining case where this could happen, when a commit
			// tracks a new tree. The commitFrom is a relative commit, because
			// when we receive the GitHub event, there's no indication that it's a
			// new tree. But we can't fetch the diff because there's no history for
			// this commit so GitHub sends a 404.
			logger_.With("error", err).Error("cannot get diff from VCS")
		case diffReader != nil:
			defer diffReader.Close()

			patches, err = DiffIssues(r.Context(), diffReader, analysis.Issues())
			if err != nil {
				logger_.With("error", err).Error("cannot diff issues from VCS diff")
				web.errorHandler(w, r, http.StatusInternalServerError, "Could not read VCS")
				return
			}
		}
	}

	var page = struct {
		Title       string
		Analysis    *db.Analysis
		Patches     []Patch
		Outputs     []db.Output
		TotalIssues int
	}{
		Title:       "Analysis",
		Analysis:    analysis,
		Patches:     patches,
		Outputs:     outputs,
		TotalIssues: len(analysis.Issues()),
	}

	if err := web.templates.ExecuteTemplate(w, "analysis.tmpl", page); err != nil {
		logger_.With("error", err).Error("cannot parse analysis template")
	}
}
