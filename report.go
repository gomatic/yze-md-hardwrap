package hardwrap

// Aggregating a run: every finding a walk produces passes through ONE counter,
// ONE limit and ONE truncation notice, and a file the gate cannot read becomes a
// finding rather than the run's error — because a gate that reports nothing is
// indistinguishable from a clean one, and that is the only outcome it must never
// produce.

import (
	"fmt"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
)

// ErrReadFile reports a document that could not be read.
const ErrReadFile errs.Const = "cannot read documentation file"

// ErrNotRegularFile reports a named path whose contents cannot be read as a
// document. It IS the shared sentinel rather than a local copy: the refusal is
// raised by the discovery this command walks with, and a caller matching a local
// copy would match only for as long as the two happened to carry the same text.
const ErrNotRegularFile = goyze.ErrNotRegularFile

// ErrNoPaths reports a run given nothing to analyze. A runner whose root
// placeholder expands to nothing would otherwise green the gate over a
// repository no analyzer ever looked at.
const ErrNoPaths errs.Const = "no paths to analyze"

// unreadableMessage formats the finding for a document that could not be read
// as prose.
const unreadableMessage = "cannot be analyzed as a document: %v; the gate cannot vouch for a file it could not " +
	"read, so this is reported rather than passed over"

// FileReader reads a file's bytes; injected so aggregation is testable without
// the filesystem.
type FileReader func(path string) ([]byte, error)

// Report runs the rule over each document and aggregates the diagnostics into
// the lean stickler-json report.
//
// unreadable is what the walk could not read: a directory it could not enter,
// and a file it could not have read had it tried — a FIFO, a device, a link
// resolving to nothing. One this run WOULD have read is REPORTED rather than
// skipped, because a document the gate cannot open is where an unchecked one
// would hide, and it is reported THROUGH this function rather than beside it:
// prepending those findings after the fact put them outside the run's limit, so
// a tree of unreadable directories was the one unbounded path left in a bounded
// report.
//
// WHICH of them arrive here is the caller's decision and not this function's,
// and that split is deliberate. The shared walk reaches its unreadable arms
// BEFORE it asks whether an analyzer claims the path, so the list it hands back
// carries entries of any name — a live ssh-agent socket and a dangling `.go`
// symlink were two of the error-severity `yze/hardwrap` findings a fleet sweep
// produced, and neither has a remedy an author can take. Bounding that needs
// two facts: whether this run would have read the path, which is the analyzer's
// [Documents], and whether the path is a TREE nobody looked inside, which only
// the filesystem knows. The command holds both — see its `reportable` — and
// this function reports what it is given, so one limit still governs everything
// the run emits. Found by an adversarial review.
func Report(read FileReader, files, unreadable []string, settings Settings) goyze.Report {
	// Started EMPTY rather than nil, so a clean run encodes as
	// `{"diagnostics":[]}` — the same shape as a run with findings, rather than
	// a null a consumer has to special-case.
	run := collecting{found: []goyze.Diagnostic{}}
	for _, path := range unreadable {
		run = run.add(Path(path), []goyze.Diagnostic{unreadableFinding(Path(path), nil)}, 1)
	}
	for _, file := range files {
		found, held := fileFindings(read, Path(file), settings)
		run = run.add(Path(file), found, held)
	}
	return run.done()
}

// collecting is one run in progress: what it has collected, how much it has
// FOUND, and the file it stopped collecting at.
//
// The two numbers are separate because they answer different questions. A
// document past its own limit hands back a truncated slice, so summing the
// slices would count this run's truncation instead of the documents' findings —
// and past the run's limit it keeps counting while it stops collecting, so the
// total it finally reports is the true one.
type collecting struct {
	truncatedAt Path
	found       []goyze.Diagnostic
	total       findingCount
}

// add takes one path's findings into the run.
func (c collecting) add(file Path, found []goyze.Diagnostic, held findingCount) collecting {
	c.total += held
	if c.truncatedAt != "" {
		return c
	}
	room := reportLimit - findingCount(len(c.found))
	if findingCount(len(found)) > room {
		c.found = append(c.found, found[:room]...)
		c.truncatedAt = file
		return c
	}
	c.found = append(c.found, found...)
	return c
}

// done is the finished report, with the notice that stands for anything the
// limit dropped.
func (c collecting) done() goyze.Report {
	if c.truncatedAt != "" {
		c.found = append(c.found, runTruncation(c.truncatedAt, c.total))
	}
	return goyze.Report{Diagnostics: c.found}
}

// fileFindings is one file's diagnostics, whether it could be read or not.
//
// A file the gate fails to open becomes ONE finding against that file and the
// run continues, as an unparseable one does, so a single blob mis-claimed by
// discovery leaves every other file's findings intact.
func fileFindings(read FileReader, file Path, settings Settings) ([]goyze.Diagnostic, findingCount) {
	data, err := read(string(file))
	if err != nil {
		return []goyze.Diagnostic{unreadableFinding(file, err)}, 1
	}
	return documentDiagnostics(file, Source(data), settings)
}

// documentDiagnostics is one document's findings, with a document too large,
// too deeply nested or not text reported as a finding of its own rather than
// raised as the whole run's error.
func documentDiagnostics(file Path, source Source, settings Settings) ([]goyze.Diagnostic, findingCount) {
	diags, held, err := countedDiagnostics(file, source, settings)
	if err != nil {
		return []goyze.Diagnostic{diagnostic(file, 1, finding(fmt.Sprintf(unreadableMessage, err)))}, 1
	}
	return diags, held
}

// runTruncation is the finding that stands for everything past the run's limit,
// carrying the true total so nothing is silently lost.
//
// It is attributed to the FILE the run stopped collecting at, not to no file at
// all: a diagnostic with an empty path is one the runner can neither baseline
// nor attribute, and this one has a truthful path available.
func runTruncation(at Path, found findingCount) goyze.Diagnostic {
	return diagnostic(at, 1, finding(fmt.Sprintf(runTruncationMessage, found, reportLimit, at)))
}

// unreadableFinding is the finding for a path the analyzer could not open at all.
func unreadableFinding(file Path, cause error) goyze.Diagnostic {
	return diagnostic(file, 1, finding(fmt.Sprintf(unreadableMessage, readFailure(file, cause))))
}

// readFailure is the error a path the gate could not open produces. It is a
// value rather than a formatted string so the sentinel it carries is matchable
// with errors.Is — a sentinel that only ever reaches a message could be swapped
// for any other with the suite green.
func readFailure(file Path, cause error) error {
	return ErrReadFile.With(cause, "path", string(file))
}
