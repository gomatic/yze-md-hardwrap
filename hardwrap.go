// Package hardwrap reports a paragraph, list item, blockquote, table row or
// heading whose SOURCE spans more than one line.
//
// The standard it mechanizes is one sentence: write each of those as ONE
// physical line, however long. A renderer decides where a rendered line ends;
// a newline inside a block is therefore a decision about the SOURCE, and it
// costs a diff line per reflow, an inconsistent line length per paragraph, and
// a merge conflict per neighbouring edit.
//
// That is what makes this rule EXACT rather than heuristic. It is not "lines
// look short here" — it is "this block occupies more than one line", which a
// parse answers with certainty. The parse is the whole design: every construct
// that LOOKS like a run of wrapped prose and is not one — a table's rows,
// consecutive list items, a fenced example, front matter — is simply a
// different node.
//
// # Strict by default
//
// A run reports EVERY line ending inside a block, whatever the source did to
// spell it. The format's own spellings for a break meant to be seen are not
// exempt, because an exemption available per line is not an exemption — it is
// the rule's off switch, reachable one line at a time by anyone who learns it,
// and a rule with an off switch measures how well a writer knows the switch.
//
// A document whose breaks really are authored — a poem, an address, a
// transcript — is served by [AuthoredBreaks], which is a deliberate, reviewable
// line in that repository's own configuration rather than a per-block escape.
// See [Settings].
//
// The one construct exempt in every run is a GitHub alert marker (`> [!NOTE]`
// and its four siblings), which is not a spelling of a line ending at all: the
// marker must stand alone on its blockquote's first line for the alert to
// exist, so joining it deletes the construct rather than reflowing it.
//
// # What this rule does not see
//
// Three limits are known, measured and accepted rather than guessed at:
//
//   - A newline inside a multi-line code SPAN is not reported. The parser hands
//     back one inline node for the whole span, so the break inside it is not a
//     break between two pieces of the block's text.
//   - A CR-only line ending (a pre-OSX Macintosh document) is not a line ending
//     to a CommonMark parser, so such a document is one long line and reports
//     nothing. Fleet count: zero.
//   - A Hugo shortcode (`{{< tabs >}}`) is ordinary paragraph text to a
//     CommonMark parser, so a shortcode broken across lines is reported as a
//     wrapped paragraph. Measured fleet-wide before it was left alone: zero
//     shortcode use, so mechanizing it would have cost every run something to
//     detect nothing.
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

// ErrTooDeep reports a document whose containers nest past what this rule will
// parse. See [nestingLimit] for why a bound on bytes is not a bound on cost.
const ErrTooDeep errs.Const = "document nests deeper than this rule reads"

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
// joined within its marker — and it names how many source lines the block
// spans, because that is the size of the edit being asked for.
//
// It states the RULE and nothing else. A diagnostic is read at the exact moment
// its reader is looking for the cheapest way to make the gate green, so a
// message that named a spelling the rule leaves alone would be a bypass
// tutorial delivered by the tool itself, to the one audience most likely to
// apply it mechanically to every line it touches. The same discipline binds
// every string this command prints.
const wrapMessage = "this %s spans %d source lines: a paragraph, list item, blockquote, table row or heading is " +
	"written as ONE physical line, however long, because the renderer decides where a rendered line ends and " +
	"a newline here only costs a diff line per reflow and a conflict per neighbouring edit; join them"

// truncationMessage formats the finding that stands for the ones not reported.
const truncationMessage = "%d hard-wrapped blocks in this document, of which %d are reported; a document with " +
	"this many is one problem rather than that many, and reporting them all costs more memory than reading it did"

// Diagnostics reports the hard-wrapped blocks of one document.
//
// settings decides whether this path is prose at all — markdown by extension, and
// anything else only where the run was configured to read it — and how a line
// ending is judged. A path the run does not read yields no findings and no
// error: it is not this rule's business, which is a different answer from "it
// is clean".
//
// A document that is not text yields [ErrNotText], so the caller surfaces a
// tool failure rather than a clean pass over a file nobody read.
func Diagnostics(at Path, source Source, settings Settings) ([]goyze.Diagnostic, error) {
	diags, _, err := countedDiagnostics(at, source, settings)
	return diags, err
}

// countedDiagnostics is [Diagnostics] with the TRUE number of findings the
// document holds, which is not the number reported: the per-document limit
// truncates the slice, so a run summing the slices would count its own
// truncation rather than the documents.
func countedDiagnostics(at Path, source Source, settings Settings) ([]goyze.Diagnostic, findingCount, error) {
	text, err := readable(at, source)
	if err != nil {
		return nil, 0, err
	}
	if !settings.Documents.Reads(at) {
		return nil, 0, nil
	}
	found := wrapped(at, text, settings.Breaks)
	total := findingCount(len(found))
	if total > findingLimit {
		return append(found[:findingLimit], truncation(at, total)), total, nil
	}
	return found, total, nil
}

// readable is the document's text once it is known to be text at all: within
// the size bound, within the nesting bound, valid UTF-8, and with the byte order
// mark removed.
func readable(at Path, source Source) (Source, error) {
	if goyze.ByteCount(len(source)) > SizeLimit {
		return "", ErrTooLarge.With(nil, "path", string(at), "bytes", len(source))
	}
	if !utf8.ValidString(string(source)) {
		return "", ErrNotText.With(nil, "path", string(at))
	}
	text := Source(strings.TrimPrefix(string(source), byteOrderMark))
	if depth := deepest(text); depth > nestingLimit {
		return "", ErrTooDeep.With(nil, "path", string(at), "depth", int(depth))
	}
	return text, nil
}

// nestingDepth is how many blockquote markers a line opens.
type nestingDepth int

// nestingLimit bounds how deeply a document may nest before this rule declines
// to parse it.
//
// The SIZE bound does not bound the COST. A markdown parser opens one block
// parser per container per line, so a document of nested blockquotes costs
// roughly the square of its depth: 100,000 levels in 200 KB took six seconds,
// and at the size limit the same shape would run for hours — a checked-in file
// that hangs the gate, and a gate that can be hung is a gate that gets disabled.
//
// The number is twenty times the deepest nesting in the fleet's 4,859 markdown
// files, which is THREE. It refuses only a document written to be refused, and
// it refuses it as a FINDING rather than as a silence.
const nestingLimit nestingDepth = 64

// deepest is the greatest blockquote nesting any line of a document opens.
func deepest(text Source) nestingDepth {
	worst := nestingDepth(0)
	for {
		current, rest, ok := nextLine(text)
		if !ok {
			return worst
		}
		text = rest
		if depth := markerDepth(current); depth > worst {
			worst = depth
		}
	}
}

// markerDepth is how many blockquote markers open one line. Only the leading
// run counts — a `>` in prose is a greater-than sign, not a container.
func markerDepth(text line) nestingDepth {
	depth := nestingDepth(0)
	for _, marker := range text {
		if marker == '>' {
			depth++
			continue
		}
		if marker != ' ' && marker != '\t' {
			return depth
		}
	}
	return depth
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
