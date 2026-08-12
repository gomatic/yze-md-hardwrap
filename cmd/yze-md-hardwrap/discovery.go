package main

// What this command claims: what counts as a document, and whose prose is
// somebody else's. Everything else about turning arguments into files — the
// symlinked root, the identity of a path reached two ways, the tree that cannot
// be read, the size bound, the ignore filter — is the shared discovery's,
// because analyzers answering those questions separately answered them
// differently.

import (
	goyze "github.com/gomatic/go-yze"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// discovery is this command's file discovery: the shared walk, told what a
// document is and whose trees to skip.
//
// What it claims is the SAME question the analyzer answers, asked of the same
// value: a walk that claimed a file the analyzer then declined to read would
// report nothing about it and say nothing about having skipped it.
func discovery(docs hardwrap.Documents) goyze.Discovery {
	return goyze.Discovery{Files: files, Claims: claimed(docs), Prunes: pruned}
}

// claimed is the walk's file test, bound to this run's configured document
// kinds.
func claimed(docs hardwrap.Documents) goyze.Claim {
	return func(path goyze.FilePath) bool { return docs.Reads(hardwrap.Path(path)) }
}

// pruned reports the trees that hold somebody else's prose. A dependency's or a
// theme's documentation is its own business, and reporting it tells this
// repository to reflow a file it does not own.
//
// This list names only trees that are somebody else's in EVERY repository. What
// a particular repository ignores is git's answer, not a list's, and `.git` and
// nested checkouts are the shared walk's business. `testdata` is here for a
// reason specific to this family: it is where an analyzer is proven in both
// directions, so a fixture that MUST contain a violation would otherwise be
// reported as one.
func pruned(name goyze.DirName) bool {
	switch name {
	case "vendor", "node_modules", "themes", "testdata", ".venv", "venv", ".tox":
		return true
	}
	return false
}
