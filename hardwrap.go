// Package hardwrap reports prose whose SOURCE spans more lines than it renders.
//
// A newline inside a paragraph, a list item, a blockquote or a heading that is
// not an explicit hard break — a trailing backslash, or two trailing spaces —
// is invisible in the rendered output. Every CommonMark renderer joins those
// lines. Such a newline therefore cannot carry authorial meaning; it exists
// only to fit a column, and it costs a diff line per reflow, an inconsistent
// line length per paragraph, and a merge conflict per neighbouring edit.
//
// That is what makes this rule EXACT rather than heuristic. It is not "lines
// look short here" — it is "the renderer discards this newline", which the
// parser answers with certainty. The escape hatch already exists in the format
// itself: an author who wants a visible break writes one, and a line ending in
// a backslash or two spaces is never reported.
//
// The whole rule is therefore one sentence of code: every soft line break the
// markdown parser reports is a hard-wrapped line. Everything else in this
// package exists to decide which files are prose, which regions of them are
// prose, and how much of a run's findings one report may carry.
package hardwrap

import (
	"fmt"
	"strings"
	"unicode/utf8"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
)

// Name is the analyzer's stable identifier — the suffix of its flat rule id and
// the key the yze suite catalogs it under.
const Name = "hardwrap"

// Tool is the suite name stamped on every diagnostic.
const Tool = "yze"

// Rule is the stable, flat rule id every diagnostic carries: "yze/" + [Name].
const Rule = Tool + "/" + Name

// Category is the language group this analyzer belongs to, used by the yze
// suite to run it only when processing documentation.
const Category = "docs"

// ErrTooLarge reports a file past the size this rule will read. It IS the shared
// sentinel rather than a second one beside it: the bound is enforced in two
// places — here, and at the one place the command reads a file — and two
// sentinels for one condition make `errors.Is` answer false for whichever layer
// a caller had not thought of.
const ErrTooLarge = goyze.ErrTooLarge

// ErrNotText reports a document whose bytes are not text. A markdown parser
// given arbitrary bytes still produces blocks, so a binary file would yield
// findings invented from whatever byte happened to end a line.
const ErrNotText errs.Const = "document is not valid UTF-8 text"

// SizeLimit is the largest document read, in bytes. It is exported so the
// command can refuse a file from its directory entry, BEFORE opening it —
// asking afterwards costs the file's own size for a rule that then declines to
// apply. It is generous by three orders of magnitude over any real document, so
// it bounds the pathological case without ever turning away prose.
const SizeLimit goyze.ByteCount = 8 << 20

// byteOrderMark is the invisible prefix a Windows editor writes. It is stripped
// ONCE, before anything reads the text, so the parser is not handed an
// invisible character at the top of the very line a generated claim has to be
// on.
const byteOrderMark = "\ufeff"

// Path is the file path stamped on each diagnostic's location.
type Path string

// Source is the text of one document.
type Source string

// line is one physical line of a document, without its newline.
type line string

// finding is one diagnostic's rendered message.
type finding string

// lineNumber is a 1-based position in a document.
type lineNumber int

// findingCount is how many findings a document produced.
type findingCount int

// findingLimit bounds how many findings ONE document contributes.
//
// The parser holds the whole document in memory already; the diagnostics do not
// have to. A pathological input — eight megabytes of two-line paragraphs — is
// hundreds of thousands of findings, each carrying its own message and path,
// and the report then costs an order of magnitude more than the document did.
// No author needs the ten-thousandth instance to act, and a document with this
// many is one problem, not ten thousand.
const findingLimit findingCount = 1000

// wrapMessage formats a hard-wrapped block. It names the construct because the
// fix differs in shape between them — a paragraph is joined, a list item is
// joined within its marker — even though the defect is the same newline.
const wrapMessage = "this %s is hard-wrapped: markdown joins these lines, so the newline is invisible in the " +
	"rendered output and exists only to fit a column; write it as one line, or end this line with a " +
	"backslash where the break is meant to be seen"

// truncationMessage formats the finding that stands for the ones not reported.
const truncationMessage = "%d hard-wrapped blocks in this document, of which %d are reported; a document with " +
	"this many is one problem rather than that many, and reporting them all costs more memory than reading it did"

// Diagnostics reports the hard-wrapped blocks of one document.
//
// docs decides whether this path is prose at all: markdown by extension always,
// and anything else only where the run was configured to read it. A path the
// run does not read yields no findings and no error — it is not this rule's
// business, which is a different answer from "it is clean".
//
// A document that is not text yields [ErrNotText], so the caller surfaces a
// tool failure rather than a clean pass over a file nobody read.
func Diagnostics(at Path, source Source, docs Documents) ([]goyze.Diagnostic, error) {
	diags, _, err := countedDiagnostics(at, source, docs)
	return diags, err
}

// countedDiagnostics is [Diagnostics] with the TRUE number of findings the
// document holds, which is not the number reported: the per-document limit
// truncates the slice, so a run summing the slices would count its own
// truncation rather than the documents.
func countedDiagnostics(at Path, source Source, docs Documents) ([]goyze.Diagnostic, findingCount, error) {
	text, err := readable(at, source)
	if err != nil {
		return nil, 0, err
	}
	if !docs.Reads(at) {
		return nil, 0, nil
	}
	found := wrapped(at, text)
	total := findingCount(len(found))
	if total > findingLimit {
		return append(found[:findingLimit], truncation(at, total)), total, nil
	}
	return found, total, nil
}

// readable is the document's text once it is known to be text at all: within
// the size bound, valid UTF-8, and with the byte order mark removed.
func readable(at Path, source Source) (Source, error) {
	if goyze.ByteCount(len(source)) > SizeLimit {
		return "", ErrTooLarge.With(nil, "path", string(at), "bytes", len(source))
	}
	if !utf8.ValidString(string(source)) {
		return "", ErrNotText.With(nil, "path", string(at))
	}
	return Source(strings.TrimPrefix(string(source), byteOrderMark)), nil
}

// truncation is the finding that replaces everything past the limit, so the
// count is never silently lost.
func truncation(at Path, found findingCount) goyze.Diagnostic {
	return diagnostic(at, 1, finding(fmt.Sprintf(truncationMessage, found, findingLimit)))
}

// diagnostic builds one finding at a line. Every finding of this rule addresses
// a whole line — the line whose ending newline the renderer discards — so the
// column is always the first, which is where an editor should land to join it.
func diagnostic(at Path, where lineNumber, message finding) goyze.Diagnostic {
	return goyze.Diagnostic{
		Tool:     Tool,
		Rule:     Rule,
		Path:     string(at),
		Line:     int(where),
		Col:      1,
		Severity: goyze.SeverityError,
		Message:  string(message),
	}
}
