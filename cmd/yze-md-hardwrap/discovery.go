package main

// What this command claims: what counts as a document, and whose prose is
// somebody else's. Everything else about turning arguments into files — the
// symlinked root, the identity of a path reached two ways, the tree that cannot
// be read, the size bound, the ignore filter — is the shared discovery's,
// because analyzers answering those questions separately answered them
// differently.

import (
	"errors"
	"io/fs"

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

// reportable is the paths the walk could not read that this rule has anything
// to say about: a document it would have read, and a TREE nobody looked inside.
//
// The shared walk hands back one flat list, and it reaches its unreadable arms
// BEFORE it asks whether an analyzer claims the path — so the list carries
// entries of any name, and every one of them became an error-severity
// `yze/hardwrap` finding. A fleet sweep produced two of them on a live ssh-agent
// socket and a dangling `.go` symlink: a rule about where a paragraph's newline
// goes has nothing to say about a unix socket, the remedy is unavailable at any
// price — an agent socket cannot be made readable as markdown and deleting a
// live one is not an edit — and the notice carries the same flat rule id as the
// wrap finding, so the only silence available was a `.stickler.yaml` entry that
// also switched off every genuine hard wrap in the repository. That is the
// disablement an unactionable finding manufactures. Found by an adversarial
// review.
//
// A TREE is kept whatever it is called, because a directory nobody entered is
// where an unchecked document hides, and no document set claims a directory by
// name. The two are told apart by ASKING THE FILESYSTEM rather than by reading
// the path: a name is not evidence of a kind, and `locked` and `pipe.sock` are
// spelled alike enough that any rule about their spelling would be a guess.
func reportable(unreadable []string, docs hardwrap.Documents) []string {
	kept := make([]string, 0, len(unreadable))
	for _, path := range unreadable {
		if docs.Reads(hardwrap.Path(path)) || mayHoldDocuments(hardwrap.Path(path)) {
			kept = append(kept, path)
		}
	}
	return kept
}

// mayHoldDocuments reports a path this rule must speak about whatever it is
// named: a directory, and a path the filesystem will not answer for AT ALL.
//
// The second half is the one that matters, and leaving it out was a regression
// an adversarial review caught. A tree is normally unenterable because its own
// mode denies it, and stat then answers fine — but a directory read WITHOUT the
// execute bit lists its children and lets nothing stat them, so the walk hands
// back `a/b` and both stat and lstat fail with EACCES. Deciding "not a
// directory, so not our business" from a failed stat therefore dropped an
// entire subtree, and the hard-wrapped documents inside it went unanalyzed and
// unmentioned — the one outcome a gate must never produce, reached through the
// arm added to stop a socket being reported.
//
// So the question is not "is it a directory" but "can the filesystem rule it
// out", and only a DEFINITE no drops the path. The two failures are not the
// same answer: "there is nothing here" settles the question, and "I am not
// allowed to look" settles nothing, so the first drops and the second keeps.
// Collapsing them was the regression, and collapsing them the other way — keep
// on any error — would report every path that has since been deleted.
//
// lstat is asked first because it answers for the LINK, and stat second because
// it answers for what the link leads to: a link resolving to nothing is
// definitely not a tree, while a link to a directory is one. The walk does not
// appear to hand back a link to a directory today — measured on both binaries,
// a symlinked directory inside a walked tree reaches no list at all — so that
// arm is written for what stat means rather than for a path observed reaching
// it.
func mayHoldDocuments(at hardwrap.Path) bool {
	if _, err := files.Lstat(string(at)); err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	info, err := files.Stat(string(at))
	return err == nil && info.IsDir()
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
