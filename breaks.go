package hardwrap

// The rule itself: a real CommonMark parse, and the line breaks it reports.
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
// parsed, where its lines begin, how many lines were taken off its top — so a
// position from the parser becomes the line an author's editor shows — and how
// this run was told to read a line ending.
type scanned struct {
	source []byte
	lines  lineIndex
	offset lineOffset
	breaks Breaks
}

// wrapped is every hard-wrapped block of one document, in source order.
func wrapped(at Path, text Source, breaks Breaks) []goyze.Diagnostic {
	body, offset := withoutFrontMatter(text)
	doc := scanned{source: []byte(body), lines: newLineIndex(body), offset: offset, breaks: breaks}
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
// reported break, which is where an editor lands to make that edit, and it
// carries how many source lines the wrapped region spans, which is what the
// author is being asked to make one.
func blockFinding(at Path, node ast.Node, doc scanned) []goyze.Diagnostic {
	breaks := doc.reported(node, lineBreaks(node, nil))
	if len(breaks) == 0 {
		return nil
	}
	first, last := doc.lines.of(breaks[0]), doc.lines.of(breaks[len(breaks)-1])
	message := fmt.Sprintf(wrapMessage, blockName(node), lineCount(last-first)+spannedBeyondTheLastBreak)
	return []goyze.Diagnostic{diagnostic(at, first+lineNumber(doc.offset), finding(message))}
}

// lineCount is how many source lines a block's wrapped region spans.
type lineCount int

// spannedBeyondTheLastBreak turns a distance between break LINES into a count
// of lines: the first and the last break line are the same line when a block
// wraps once, and the line the last break runs onto is spanned as well.
const spannedBeyondTheLastBreak lineCount = 2

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

// lineBreak is one newline inside a block's own text, and how the FORMAT spells
// it. Both spellings are collected and the choice between them is made later,
// because which of them a run reports is configuration — and a break dropped
// here would be a decision the configuration could no longer reach.
type lineBreak struct {
	at     byteOffset
	isHard hardBreak
}

// hardBreak is the parser's answer about one line ending: a spelling the format
// renders as a visible break, rather than a newline it discards.
type hardBreak bool

// lineBreaks is every newline inside a block's own text, appended to what it
// was given.
//
// Only the INLINE subtree is read, and that IS a correctness guard: a container
// block's prose belongs to the leaf blocks inside it, each of which is visited
// in its own right, so crossing into one would attribute a paragraph's break to
// the blockquote around it as well and report one wrap twice.
func lineBreaks(node ast.Node, into []lineBreak) []lineBreak {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if blockNodes[child.Type()] {
			continue
		}
		into = lineBreaks(child, append(into, breakOf(child)...))
	}
	return into
}

// breakOf is where this inline node's trailing newline is, when it ends a line
// at all — at most one position, expressed as a slice so a caller composes it
// without a branch.
//
// The hard spelling is asked FIRST, so that a line ending the format renders
// keeps that spelling even if a parser were to mark both: a run configured to
// leave authored breaks alone would otherwise leave alone whichever of the two
// happened to be asked about first.
func breakOf(node ast.Node) []lineBreak {
	text, isText := node.(*ast.Text)
	if !isText {
		return nil
	}
	if text.HardLineBreak() {
		return []lineBreak{{at: byteOffset(text.Segment.Start), isHard: true}}
	}
	if text.SoftLineBreak() {
		return []lineBreak{{at: byteOffset(text.Segment.Start)}}
	}
	return nil
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
