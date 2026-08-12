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
// its own, so a term cannot span lines and an entry for it would name a finding
// nothing can produce.
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
	for _, block := range blocks(document) {
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
	breaks := doc.invisible(softBreaks(node))
	if len(breaks) == 0 {
		return nil
	}
	where := doc.lines.of(breaks[0]) + lineNumber(doc.offset)
	return []goyze.Diagnostic{diagnostic(at, where, finding(fmt.Sprintf(wrapMessage, blockName(node))))}
}

// invisible is the breaks that really are invisible, which is not quite the set
// the parser calls soft.
//
// goldmark decides a backslash break from the last two characters of the line,
// so it reads an ODD run of three or more backslashes as escaped text rather
// than as an escaped backslash followed by a break. CommonMark — and therefore
// what a reader sees on GitHub or on a Hugo page — resolves the escapes first,
// and `\\\` at a line end is a literal backslash and then a VISIBLE break.
// Trusting the parser alone would report the one shape whose whole purpose is
// to make the break seen. Found by the fuzz target, on `0\\\`.
func (d scanned) invisible(breaks []byteOffset) []byteOffset {
	kept := make([]byteOffset, 0, len(breaks))
	for _, at := range breaks {
		if trailingBackslashes(d.lineAt(at))%2 == 0 {
			kept = append(kept, at)
		}
	}
	return kept
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

// blocks is every block node beneath a node, in source order.
//
// The descent stops at inline nodes because a block's inline children belong to
// that block alone; collecting them here would attribute a paragraph's breaks to
// the blockquote around it as well, and report one wrap twice.
func blocks(root ast.Node) []ast.Node {
	var found []ast.Node
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Type() != ast.TypeBlock {
			continue
		}
		found = append(append(found, child), blocks(child)...)
	}
	return found
}

// softBreaks is where a block's own text ends a line with a newline the
// renderer discards.
//
// Only the INLINE subtree is read, for the reason [blocks] descends the way it
// does: a container block's prose belongs to the leaf blocks inside it, each of
// which is visited in its own right.
func softBreaks(node ast.Node) []byteOffset {
	var found []byteOffset
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if child.Type() == ast.TypeBlock {
			continue
		}
		found = append(append(found, softBreakOf(child)...), softBreaks(child)...)
	}
	return found
}

// softBreakOf is where this inline node's trailing newline is, when it has one
// the renderer discards — at most one position, expressed as a slice so a
// caller composes it without a branch.
//
// The parser has already decided the only question that matters. A line ending
// in a backslash or in two spaces carries a HARD break, which is a break the
// author asked to be seen, and goldmark marks those separately; what is left is
// exactly the newline that exists to fit a column.
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
