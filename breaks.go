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
// line each, but whose cells are otherwise ordinary paragraph text) and Footnote
// gives `[^1]: …` definitions. Without them, consecutive definitions are ONE
// paragraph spanning several lines — a finding on a document that is correct.
//
// DEFINITION LISTS ARE DELIBERATELY ABSENT, and that is a reversal. The
// extension was here so a `Term` / `: value` pair would not read as a wrapped
// paragraph — but under it, EVERY line above a `:` is a term of its own, so
// appending one `: note` line turned twenty lines of hard-wrapped prose into
// twenty one-line terms and reported nothing. A second adversarial review found
// it. Measured before it was removed (2026-08-12): the whole fleet holds ZERO
// definition lists, in any repository, so the exemption protected nothing and
// cost a bypass anyone could type. GitHub renders the same text as one paragraph
// in the first place, which is how it is now read here.
var markdown = newParser()

// newParser assembles the markdown parser.
func newParser() parser.Parser {
	return goldmark.New(goldmark.WithExtensions(extension.GFM, extension.Footnote)).Parser()
}

// blockKind is what a hard-wrapped block is called in its finding.
type blockKind string

// blockNames is what each construct is called in a message. A node is named by
// its own kind, or — for the anonymous text block a tight list item and a
// definition description hold their prose in — by the kind of its parent, which
// is the construct an author would recognise.
var blockNames = map[ast.NodeKind]blockKind{
	ast.KindParagraph:    "paragraph",
	ast.KindHeading:      "heading",
	ast.KindListItem:     "list item",
	extast.KindTableCell: "table cell",
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

// wrapped is one document's hard-wrapped blocks, in source order, and how many
// there WERE — which is not the length of the slice.
//
// Past the per-document limit it keeps counting and stops collecting. Building
// the whole slice and trimming it afterwards bounded the report but not the
// memory the report was supposed to bound: an eight-megabyte document of
// two-line paragraphs allocated 195,122 diagnostics — 285 MB beyond the same
// document with nothing to report — to emit a thousand of them. Found by a
// second adversarial review, against a comment claiming the opposite.
func wrapped(at Path, text Source, breaks Breaks) ([]goyze.Diagnostic, findingCount) {
	body, offset := withoutFrontMatter(text)
	doc := scanned{source: []byte(body), lines: newLineIndex(body), offset: offset, breaks: breaks}
	document := markdown.Parse(mdtext.NewReader(doc.source))
	if isGenerated(document, doc.source, doc.lines) {
		return nil, 0
	}
	var found []goyze.Diagnostic
	total := findingCount(0)
	for _, block := range blocks(document, nil) {
		one := blockFinding(at, block, doc)
		total += findingCount(len(one))
		if findingCount(len(found)) < findingLimit {
			found = append(found, one...)
		}
	}
	return found, total
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
	breaks := doc.reported(node, doc.lineBreaks(node))
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
// [proseBlocks], which reads the lines of a leaf block and never those of the
// container around it.
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

// lineBreak is one line ending inside a block's own text: the position of the
// newline itself, which is where the line it ends can be read from.
type lineBreak struct {
	at byteOffset
}

// lineBreaks is every line ending inside ONE block's own text.
//
// It is taken from the block's own LINE SEGMENTS rather than from the breaks
// the parser reports between inline nodes, and that is the difference between a
// rule and a rule with a hole in it. A parser reports a break between two pieces
// of TEXT; a newline inside a raw HTML comment, a link title or a code span
// falls inside ONE inline node, so it is no break at all to the parser — and
// `prose <!--` / `--> more prose` is then a hard-wrapped paragraph that renders
// as one line and reports nothing. Found by an adversarial review, which called
// it worse than the spellings this rule stopped exempting: this one is invisible
// in the rendered output as well as in the report.
//
// A block's segments are one per source line, so the boundaries between them
// are the lines the block occupies, whatever the source put on them.
func (d scanned) lineBreaks(node ast.Node) []lineBreak {
	if !proseBlocks[node.Kind()] {
		return nil
	}
	segments := node.Lines()
	breaks := make([]lineBreak, 0, max(segments.Len()-1, 0))
	for at := range segments.Len() - 1 {
		// One before the next line's start is the newline itself, which belongs
		// to the line it ends — the line an author joins.
		breaks = append(breaks, lineBreak{at: byteOffset(segments.At(at).Stop) - 1})
	}
	return breaks
}

// proseBlocks says which blocks hold PROSE — text this rule requires to occupy
// one line. Every block kind the parser and its extensions can produce is named
// rather than compared against one: a kind absent from the table is a decision
// nobody wrote down, and this table decides what the rule applies to at all.
//
// The excluded kinds fall in three groups. A CONTAINER (a list, a blockquote, a
// table, a definition list) holds no text of its own — its lines belong to the
// leaf blocks inside it, each visited in its own right, and reading a
// container's lines would report one wrap twice. A VERBATIM block (fenced code,
// indented code, raw HTML) means its lines literally, so joining them changes
// what it says. A block with no text at all (a thematic break) has nothing to
// occupy a line with.
//
// An HTML BLOCK is the one exclusion worth naming twice, because it is the one
// that holds real prose. Its lines are markup a renderer passes through
// verbatim, and the standard governs a paragraph, a list item, a blockquote, a
// table row and a heading — an HTML block is none of those. Measured before it
// was left out (2026-08-12): 1,775 multi-line HTML blocks fleet-wide, the
// first-party ones being Hugo layout blocks on `www.*` content pages, where a
// tag per line is the whole convention. Reporting them would tell an author to
// collapse markup, which is a rule nobody has written. Prose hard-wrapped inside
// one therefore goes unjudged; see the package doc.
var proseBlocks = map[ast.NodeKind]bool{
	ast.KindParagraph:       true,
	ast.KindTextBlock:       true,
	ast.KindHeading:         true,
	extast.KindTableCell:    true,
	ast.KindDocument:        false,
	ast.KindBlockquote:      false,
	ast.KindList:            false,
	ast.KindListItem:        false,
	ast.KindThematicBreak:   false,
	ast.KindCodeBlock:       false,
	ast.KindFencedCodeBlock: false,
	ast.KindHTMLBlock:       false,
	extast.KindTable:        false,
	extast.KindTableHeader:  false,
	extast.KindTableRow:     false,
	extast.KindFootnote:     false,
	extast.KindFootnoteList: false,
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
