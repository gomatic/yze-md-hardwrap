package hardwrap

// The rule itself: a real CommonMark parse, and the soft line breaks it reports.
//
// Nothing here decides what a paragraph is. That is the whole point of parsing
// rather than scanning: every construct that LOOKS like a run of wrapped prose
// and is not one — a table's rows, consecutive list items, a run of link
// reference definitions, a setext heading's underline, an HTML block, a fenced
// or indented code block, a footnote definition, a definition list — is a
// different node to the parser, and a scanner would have to re-decide each of
// them, differently, and be wrong somewhere.

import (
	"fmt"
	"sort"
	"strings"

	goyze "github.com/gomatic/go-yze"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	mdtext "github.com/yuin/goldmark/text"
)

// markdown is the parser every document is read with.
//
// It is built ONCE. A parser is an immutable binding — the extension chain is
// assembled at construction and never mutated — and rebuilding it per document
// would assemble the same block and inline parser tables for every file in a
// fleet-wide walk.
//
// The extensions are not decoration; each one is a block shape that would
// otherwise be misread as wrapped prose. GFM gives tables (whose rows are one
// line each, but whose cells are otherwise ordinary paragraph text), Footnote
// gives `[^1]: …` definitions, and DefinitionList gives `Term` / `: value`
// pairs. Without them, consecutive definitions and a term with its definition
// are ONE paragraph spanning several lines — a finding on a document that is
// correct. This set is also what Hugo enables by default, so a content page is
// read the way the site that publishes it reads it.
var markdown = newParser()

// newParser assembles the markdown parser.
func newParser() parser.Parser {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.DefinitionList),
	).Parser()
}

// blockKind is what a hard-wrapped block is called in its finding.
type blockKind string

// blockNames is what each construct is called in a message. A node is named by
// its own kind, or — for the anonymous text block a tight list item and a
// definition description hold their prose in — by the kind of its parent, which
// is the construct an author would recognise.
//
// A definition TERM is deliberately absent: each line above a `:` is a term of
// its own, so two lines are two terms rather than one term wrapped, and an entry
// for it would name a finding nothing produces.
var blockNames = map[ast.NodeKind]blockKind{
	ast.KindParagraph:                "paragraph",
	ast.KindHeading:                  "heading",
	ast.KindListItem:                 "list item",
	extast.KindTableCell:             "table cell",
	extast.KindDefinitionDescription: "definition",
}

// scanned is one document as everything below reads it: the bytes that were
// parsed, where its lines begin, and how many lines were taken off its top — so
// a position from the parser becomes the line an author's editor shows.
type scanned struct {
	source []byte
	lines  lineIndex
	offset lineOffset
}

// wrapped is every hard-wrapped block of one document, in source order.
func wrapped(at Path, text Source) []goyze.Diagnostic {
	body, offset := withoutFrontMatter(text)
	doc := scanned{source: []byte(body), lines: newLineIndex(body), offset: offset}
	document := markdown.Parse(mdtext.NewReader(doc.source))
	if isGenerated(document, doc.source, doc.lines) {
		return nil
	}
	var found []goyze.Diagnostic
	for _, block := range blocks(document, nil) {
		found = append(found, blockFinding(at, block, doc)...)
	}
	return found
}

// blockFinding is the ONE finding a block earns, or none.
//
// One finding per block rather than one per break, because the fix is one
// edit — the block is joined — and a paragraph wrapped at eighty columns over
// twenty lines is one mistake, not nineteen. It is positioned at the FIRST
// invisible break, which is where an editor lands to make that edit.
func blockFinding(at Path, node ast.Node, doc scanned) []goyze.Diagnostic {
	breaks := doc.invisible(node, softBreaks(node, nil))
	if len(breaks) == 0 {
		return nil
	}
	where := doc.lines.of(breaks[0]) + lineNumber(doc.offset)
	return []goyze.Diagnostic{diagnostic(at, where, finding(fmt.Sprintf(wrapMessage, blockName(node))))}
}

// invisible is the breaks that really are invisible, which is not quite the set
// the parser calls soft.
func (d scanned) invisible(node ast.Node, breaks []byteOffset) []byteOffset {
	kept := make([]byteOffset, 0, len(breaks))
	for _, at := range breaks {
		if !d.isVisible(node, at) {
			kept = append(kept, at)
		}
	}
	return kept
}

// isVisible reports a break a READER sees, which the parse alone does not
// answer. Two shapes are visible while goldmark calls their newline soft, and
// each is a break whose whole purpose is to be seen — reporting one tells an
// author to destroy what they wrote.
func (d scanned) isVisible(node ast.Node, at byteOffset) bool {
	text := d.lineAt(at)
	return trailingBackslashes(text)%2 == 1 || d.opensAlert(node, at, text)
}

// alertMarkers are the five GitHub alert types. The marker must stand ALONE on
// the first line of its blockquote, so the newline after it is structural: join
// it and the alert becomes an ordinary quotation, losing its icon, its colour
// and its meaning. It is invisible in plain CommonMark and visible everywhere
// this fleet's markdown is actually rendered.
//
// Measured before it was mechanized (2026-08-12): nine markers in seven files
// fleet-wide, every one of them third-party — a Hugo theme, vendored provider
// changelogs, spec-kit command templates — and two of them were reported. A
// false positive that instructs an author to break a correct document is worth
// ten lines whatever its count, and this construct is common everywhere else.
var alertMarkers = map[line]bool{
	"[!NOTE]": true, "[!TIP]": true, "[!IMPORTANT]": true, "[!WARNING]": true, "[!CAUTION]": true,
}

// opensAlert reports the marker line of a GitHub alert.
//
// Three conditions, and the third was a hole an adversarial review found: the
// block sits in a blockquote (`[!NOTE]` in ordinary prose is a bracketed word,
// and its newline really is decorative), the marker is the type GitHub defines,
// and it is on the block's FIRST line. Only the first line of a blockquote
// opens an alert; a second `> [!NOTE]` below it is literal body text, and
// exempting that one silenced a genuine wrap.
func (d scanned) opensAlert(node ast.Node, at byteOffset, text line) bool {
	parent := node.Parent()
	// The blockquote's own markers are stripped because the line is read from
	// the RAW document — the parser's segments have them removed already, but
	// the raw line is what a position maps back to, and `> [!NOTE]` is the
	// spelling on disk.
	quoted := strings.TrimLeft(string(text), " \t>")
	return parent != nil && parent.Kind() == ast.KindBlockquote && d.opensBlock(node, at) &&
		alertMarkers[line(strings.ToUpper(strings.TrimSpace(quoted)))]
}

// opensBlock reports a position lying on the first line of its block.
func (d scanned) opensBlock(node ast.Node, at byteOffset) bool {
	lines := node.Lines()
	return lines.Len() > 0 && d.lines.of(byteOffset(lines.At(0).Start)) == d.lines.of(at)
}

// lineAt is the text of the line holding an offset, without its ending.
func (d scanned) lineAt(at byteOffset) line {
	found := d.lines.of(at)
	start := d.lines[found-1]
	end := byteOffset(len(d.source))
	if int(found) < len(d.lines) {
		// One before the next line's start is the newline itself, which is not
		// part of the line the author wrote.
		end = d.lines[found] - 1
	}
	return line(strings.TrimSuffix(string(d.source[start:end]), "\r"))
}

// trailingBackslashes is the length of the run of backslashes a line ends with.
// An even run is escaped backslashes and ordinary text; an odd one leaves a
// lone backslash against the newline, which is a break the author asked to be
// seen.
func trailingBackslashes(text line) int {
	return len(text) - len(strings.TrimRight(string(text), `\`))
}

// blockName is what this block is called in a finding.
func blockName(node ast.Node) blockKind {
	if name, isNamed := blockNames[node.Kind()]; isNamed {
		return name
	}
	if parent := node.Parent(); parent != nil {
		if name, isNamed := blockNames[parent.Kind()]; isNamed {
			return name
		}
	}
	return "block"
}

// blocks is every block node beneath a node, in source order, appended to what
// it was given.
//
// The accumulator is threaded through the recursion rather than each level
// returning its own slice, because `append(append(found, child), blocks(child)…)`
// re-copies the whole subtree at every level of nesting — O(n·depth), which for
// a document that nests linearly is quadratic. A 200 KB file of 100,000 nested
// blockquotes took 52 seconds in that shape, so the size bound did not bound the
// COST: a checked-in file could hang the gate, and a gate that can be hung is a
// gate that gets disabled. Found by an adversarial review.
//
// The descent skips inline nodes because they hold no blocks; that is a saving,
// not a correctness guard — what keeps one wrap from being reported twice is
// [softBreaks] refusing to cross into a nested block.
func blocks(root ast.Node, into []ast.Node) []ast.Node {
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if !blockNodes[child.Type()] {
			continue
		}
		into = blocks(child, append(into, child))
	}
	return into
}

// blockNodes says which kinds of node hold a document's structure. Every member
// is named rather than compared against one: a node type absent from the table
// is a decision nobody wrote down, and this table decides what is walked.
var blockNodes = map[ast.NodeType]bool{
	ast.TypeBlock: true,
	// An inline node holds a block's own text, never another block, and a
	// document is a root rather than anybody's child.
	ast.TypeInline:   false,
	ast.TypeDocument: false,
}

// softBreaks is where a block's own text ends a line with a newline the
// renderer discards, appended to what it was given.
//
// Only the INLINE subtree is read, and that IS a correctness guard: a container
// block's prose belongs to the leaf blocks inside it, each of which is visited
// in its own right, so crossing into one would attribute a paragraph's break to
// the blockquote around it as well and report one wrap twice.
func softBreaks(node ast.Node, into []byteOffset) []byteOffset {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if blockNodes[child.Type()] {
			continue
		}
		into = softBreaks(child, append(into, softBreakOf(child)...))
	}
	return into
}

// softBreakOf is where this inline node's trailing newline is, when it has one
// the renderer discards — at most one position, expressed as a slice so a
// caller composes it without a branch.
//
// The parser has already decided the question that matters. A line ending in a
// backslash or in two spaces carries a HARD break, which is a break the author
// asked to be seen, and goldmark marks those separately; what is left is the
// newline that exists to fit a column.
func softBreakOf(node ast.Node) []byteOffset {
	text, isText := node.(*ast.Text)
	if !isText || !text.SoftLineBreak() {
		return nil
	}
	return []byteOffset{byteOffset(text.Segment.Start)}
}

// byteOffset is a position in a document's bytes.
type byteOffset int

// lineIndex is where each line of a document starts, so a byte offset from the
// parser becomes the line number an author's editor shows.
type lineIndex []byteOffset

// newLineIndex indexes a document's lines.
func newLineIndex(text Source) lineIndex {
	starts := lineIndex{0}
	for at := range len(text) {
		if text[at] == '\n' {
			starts = append(starts, byteOffset(at+1))
		}
	}
	return starts
}

// of is the 1-based line holding a byte offset. A line number is always at
// least one: a position before the start of the document is still reported
// somewhere an editor can open, never at a line zero no file has.
func (l lineIndex) of(at byteOffset) lineNumber {
	found := sort.Search(len(l), func(i int) bool { return l[i] > at })
	if found < 1 {
		return 1
	}
	return lineNumber(found)
}
