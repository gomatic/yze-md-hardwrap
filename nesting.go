package hardwrap

// The bound on NESTING, which is the bound the size limit does not provide.
//
// A markdown parser opens one block parser per container per line, so the cost
// of a document grows with the SQUARE of how deeply it nests while its byte
// count barely moves. A document written to be slow is refused here, before it
// is parsed — as a finding, never as a silence.

import (
	"strings"
)

// nestingDepth is how many containers a line opens.
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
// The number is measured against the fleet: counting blockquote markers, list
// markers and the indentation in front of a list marker, the deepest line in any
// markdown file on disk scores SEVENTEEN — inside a vendored dependency's README
// — and the deepest first-party line scores eight. So the bound leaves nearly
// four times the headroom of anything anybody has written; it refuses only a
// document written to be refused, and it refuses it as a FINDING rather than as
// a silence.
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

// markerDepth is how many containers one line opens.
//
// Only the LEADING run counts — a `>` in prose is a greater-than sign and a
// dash mid-sentence is a dash. Three things open a container there, and the
// third is the one a second adversarial review found missing: a blockquote
// marker, a LIST marker, and the INDENTATION in front of a list marker, which is
// how a deeply nested list is spelled across many lines rather than on one.
// Counting only blockquotes left `- ` repeated fifty thousand times — one line,
// 100 KB — costing sixteen seconds, and a 4 MB document of steadily indented
// list items costing eighteen. That is the same hang the bound exists to
// prevent, reached through the containers it was not counting.
func markerDepth(text line) nestingDepth {
	depth, indent, isMarked := nestingDepth(0), columns(0), false
	for rest := leading(text); rest != ""; {
		opened, width := openerAt(rest)
		if width == 0 {
			return depth + indent.levels(isMarked)
		}
		depth, isMarked = depth+opened.depth, isMarked || opened.isList
		indent += opened.indent
		rest = rest[width:]
	}
	return depth + indent.levels(isMarked)
}

// opener is what one piece of a line's leading run contributes: a container it
// opens, indentation it adds, and whether it was a LIST marker — which is what
// makes the indentation in front of it a nesting level rather than a margin.
type opener struct {
	depth  nestingDepth
	indent columns
	isList bool
}

// columns is a width of leading whitespace, in character cells.
type columns int

// spacesPerTab is what a tab is worth as indentation, which is what CommonMark
// counts it as.
const spacesPerTab columns = 4

// spacesPerLevel is how much indentation stands for one level of nesting. A list
// nested one level deeper is written two columns further in at its narrowest, so
// this is the widest a level can be counted at without under-counting depth.
const spacesPerLevel columns = 2

// levels is how many containers this much indentation stands for, which is none
// at all unless a list marker followed it. Indentation with no marker after it
// is a code block or a margin — one block however wide it is — and counting it
// would refuse a document for the width of an example.
func (c columns) levels(isMarked bool) nestingDepth {
	if !isMarked {
		return 0
	}
	return nestingDepth(c / spacesPerLevel)
}

// openerAt reads one piece of a line's leading run, reporting how wide it was.
// A width of zero ends the run: what follows is the line's own text.
func openerAt(text leading) (opener, markerWidth) {
	switch text[0] {
	case '>':
		return opener{depth: 1}, 1
	case ' ':
		return opener{indent: 1}, 1
	case '\t':
		return opener{indent: spacesPerTab}, 1
	}
	if width := listMarkerWidth(text); width > 0 {
		// Clamped to what is LEFT of the line: a marker at the very end of one is
		// an empty list item, so it opens a container while occupying less than a
		// marker and a space. Unclamped, a line of `*` alone read two bytes off a
		// one-byte line and panicked — found by the fuzz target, which is the
		// only place a single-character document was ever going to come from.
		return opener{depth: 1, isList: true}, min(width, markerWidth(len(text)))
	}
	return opener{}, 0
}

// leading is the part of a line's leading run that has not been read yet.
type leading string

// markerWidth is how many bytes one piece of a leading run occupies. Zero ends
// the run: what follows is the line's own text.
type markerWidth int

// listMarkerWidth is how wide the list marker at the front of this text is, or
// zero when there is not one. A marker is a bullet or an ordinal FOLLOWED BY A
// SPACE, which is what makes it a marker rather than a dash in a word or a
// version number in a sentence.
func listMarkerWidth(text leading) markerWidth {
	if digits := markerWidth(len(text) - len(strings.TrimLeft(string(text), decimalDigits))); digits > 0 {
		return orderedMarkerWidth(text, digits)
	}
	if strings.ContainsRune(bulletMarkers, rune(text[0])) && isFollowedBySpace(text[1:]) {
		return bulletMarkerWidth
	}
	return 0
}

// bulletMarkerWidth is a bullet and the space after it.
const bulletMarkerWidth markerWidth = 2

// orderedMarkerWidth is the width of an ordinal marker — digits, a delimiter and
// a space — or zero when the digits are just a number.
func orderedMarkerWidth(text leading, digits markerWidth) markerWidth {
	rest := text[digits:]
	if rest == "" || !strings.ContainsRune(orderedDelimiters, rune(rest[0])) || !isFollowedBySpace(rest[1:]) {
		return 0
	}
	return digits + bulletMarkerWidth
}

// isFollowedBySpace reports the space that separates a marker from its content.
// A marker at the very end of a line is still a marker: an empty list item.
func isFollowedBySpace(text leading) bool {
	return text == "" || text[0] == ' ' || text[0] == '\t'
}

// The characters a list marker is spelled with.
const (
	bulletMarkers     = "-*+"
	orderedDelimiters = ".)"
	decimalDigits     = "0123456789"
)
