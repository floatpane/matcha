// Package gitmail applies patches received as email to a local git working
// tree. It is matcha's "git-mail" feature: a message produced by
// `git format-patch` / `git send-email` can be applied to a checkout without
// shelling out to git, using floatpane's parser and applier libraries.
//
//   - github.com/floatpane/go-mailpatch parses the RFC 5322 message into commit
//     metadata and a structured diff.
//   - github.com/floatpane/go-patchapply writes the diff to a directory,
//     confined to that directory and applied transactionally.
package gitmail

import (
	"bytes"
	"fmt"

	mailpatch "github.com/floatpane/go-mailpatch"
	patchapply "github.com/floatpane/go-patchapply"
)

// Options controls how a patch is applied.
type Options struct {
	// Reverse unapplies the patch instead of applying it.
	Reverse bool
	// DryRun validates the patch against the tree but writes nothing.
	DryRun bool
}

// Summary describes the result of applying one patch message.
type Summary struct {
	// Subject is the patch subject with its "[PATCH n/m]" prefix stripped.
	Subject string
	// Author is the commit author ("Name <email>").
	Author string
	// Series is the position within a series, when the subject carried it.
	Series mailpatch.SeriesInfo
	// CoverLetter is true when the message is a "0/n" cover letter (nothing to
	// apply); Files is then empty.
	CoverLetter bool
	// Files lists what was created, updated, removed, or renamed.
	Files []patchapply.FileResult
}

func (o Options) applyOpts() *patchapply.Options {
	return &patchapply.Options{Reverse: o.Reverse, DryRun: o.DryRun}
}

func summarize(p *mailpatch.Patch, files []patchapply.FileResult) *Summary {
	return &Summary{
		Subject:     p.Subject,
		Author:      p.From,
		Series:      p.Series,
		CoverLetter: p.IsCoverLetter(),
		Files:       files,
	}
}

// Apply parses a single format-patch email (raw RFC 5322 bytes) and applies it
// to the working tree rooted at repoDir. A cover letter (a "0/n" message with
// no diff) applies cleanly as a no-op.
func Apply(raw []byte, repoDir string, opts Options) (*Summary, error) {
	p, err := mailpatch.ParseBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}
	fsys := patchapply.NewDirFS(repoDir)
	res, err := patchapply.ApplyPatch(fsys, p, opts.applyOpts())
	if err != nil {
		return nil, fmt.Errorf("apply %q: %w", p.Subject, err)
	}
	return summarize(p, res.Files), nil
}

// ApplySeries applies every patch in an mbox to repoDir in series order. The
// cover letter, if present, is summarized but applies nothing.
//
// It is not transactional across patches: if patch 3 of 5 conflicts, patches 1
// and 2 are already written. Pass Options.DryRun first to check the whole
// series, or reverse the applied prefix yourself on failure.
func ApplySeries(raw []byte, repoDir string, opts Options) ([]*Summary, error) {
	series, err := mailpatch.ParseSeries(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("parse series: %w", err)
	}

	fsys := patchapply.NewDirFS(repoDir)
	var summaries []*Summary

	if series.Cover != nil {
		summaries = append(summaries, summarize(series.Cover, nil))
	}
	for _, p := range series.Patches {
		res, err := patchapply.ApplyPatch(fsys, p, opts.applyOpts())
		if err != nil {
			return summaries, fmt.Errorf("apply [%d/%d] %q: %w",
				p.Series.Index, p.Series.Total, p.Subject, err)
		}
		summaries = append(summaries, summarize(p, res.Files))
	}
	return summaries, nil
}

// IsPatch reports whether raw looks like an applicable patch email: it parses
// and carries a diff. Use it to decide whether to offer "apply" on a message.
func IsPatch(raw []byte) bool {
	p, err := mailpatch.ParseBytes(raw)
	return err == nil && p.HasDiff()
}

// SendOptions describes how to generate and send a patch email from a local repo.
type SendOptions struct {
	// To is the primary recipient address list, comma-separated (required).
	To string
	// Cc is the carbon-copy recipient list, comma-separated.
	Cc string
	// Subject overrides the patch subject. If empty, the commit subject is used.
	Subject string
	// Version is the series revision (1 default, 2 for v2, etc.).
	Version int
	// InReplyTo is the Message-ID this patch replies to (for threading).
	InReplyTo string
	// References is the full References header value (space-separated Message-IDs).
	References string
}

// GeneratePatch generates a format-patch email from a local git repository.
// It runs `git format-patch --stdout` for the given commit range and returns
// the raw email bytes.
func GeneratePatch(repoDir, commitRange string) ([]byte, error) {
	return patchapply.GeneratePatch(repoDir, commitRange)
}

// GeneratePatchSeries generates a multi-patch mbox from a local git repository.
func GeneratePatchSeries(repoDir, commitRange string) ([]byte, error) {
	return patchapply.GeneratePatchSeries(repoDir, commitRange)
}

// FormatPatch constructs a format-patch email from structured data using
// the go-mailpatch Format function. This is useful when you have the diff
// already and want to construct a proper email without running git.
func FormatPatch(opts mailpatch.FormatOptions) ([]byte, error) {
	return mailpatch.Format(opts)
}

// ParsePatch parses raw format-patch email bytes and returns the structured Patch.
func ParsePatch(raw []byte) (*mailpatch.Patch, error) {
	return mailpatch.ParseBytes(raw)
}
