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

// Unreadable is the finding for each path the walk could not read: a directory
// it could not enter, and a file it could not have read had it tried — a FIFO,
// a device, a link resolving to nothing. Both are REPORTED rather than skipped,
// because a path the gate cannot open is where an unchecked one would hide, and
// the run still yields every other file's findings.
func Unreadable(paths []string) []goyze.Diagnostic {
	diags := make([]goyze.Diagnostic, 0, len(paths))
	for _, path := range paths {
		diags = append(diags, unreadable(Path(path), nil))
	}
	return diags
}

// Report runs the rule over each document and aggregates the diagnostics into
// the lean stickler-json report.
func Report(read FileReader, files []string, docs Documents) goyze.Report {
	report := goyze.Report{}
	total := findingCount(0)
	truncatedAt := Path("")
	for _, file := range files {
		found, held := fileFindings(read, Path(file), docs)
		// The TRUE count, not the reported one: a document past its own limit
		// hands back a truncated slice, so summing slices would count this
		// run's truncation instead of the documents' findings.
		total += held
		if truncatedAt != "" {
			// Past the limit the run keeps COUNTING but stops collecting, so
			// the total it reports is the true one.
			continue
		}
		report, truncatedAt = collect(report, found, Path(file))
	}
	if truncatedAt != "" {
		report.Diagnostics = append(report.Diagnostics, runTruncation(truncatedAt, total))
	}
	return report
}

// collect adds one document's findings to the report, reporting the file the
// run stopped collecting at when they do not all fit.
func collect(report goyze.Report, found []goyze.Diagnostic, file Path) (goyze.Report, Path) {
	room := reportLimit - findingCount(len(report.Diagnostics))
	if findingCount(len(found)) > room {
		report.Diagnostics = append(report.Diagnostics, found[:room]...)
		return report, file
	}
	report.Diagnostics = append(report.Diagnostics, found...)
	return report, ""
}

// fileFindings is one file's diagnostics, whether it could be read or not.
//
// A file the gate cannot open becomes ONE finding against that file and the run
// continues, exactly as an unparseable one does — a single blob mis-claimed by
// discovery can never take every other file's findings down with it.
func fileFindings(read FileReader, file Path, docs Documents) ([]goyze.Diagnostic, findingCount) {
	data, err := read(string(file))
	if err != nil {
		return []goyze.Diagnostic{unreadable(file, err)}, 1
	}
	return documentDiagnostics(file, Source(data), docs)
}

// documentDiagnostics is one document's findings, with a document that cannot be
// read as text reported as a finding of its own rather than raised as the whole
// run's error.
func documentDiagnostics(file Path, source Source, docs Documents) ([]goyze.Diagnostic, findingCount) {
	diags, held, err := countedDiagnostics(file, source, docs)
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

// unreadable is the finding for a document the analyzer could not open at all.
func unreadable(file Path, cause error) goyze.Diagnostic {
	return diagnostic(file, 1, finding(fmt.Sprintf(unreadableMessage, ErrReadFile.With(cause, "path", string(file)))))
}
