package hardwrap_test

// Which of a block's line breaks a run REPORTS, and the two things that decide
// it: the alert marker that is structural in every run, and the format's own
// spellings for a break meant to be seen. Every case here holds the PARSE
// constant and varies what the decision is allowed to read, because a widening
// disjunct in that one predicate is the rule's off switch and nothing else in
// this repository could see one.

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestOnlyTheFiveMarkersGitHubDefinesAreStructural pins the alert table in the
// direction nothing in this repository pinned it: ADDING an entry.
//
// The removal direction was already covered — drop a marker and the case above
// fails. The other direction was invisible: adding `"[!INFO]": true` left `make
// check` at exit 0 and total coverage 100.0%, and a two-line blockquote opening
// `> [!INFO]` went silent, although GitHub defines five alert types and renders
// no alert for INFO. The blockquote is then an ordinary quotation carrying the
// literal bracketed word, and every entry granted where GitHub renders no alert
// is a line of hard-wrapped prose nobody is told about. Found by an adversarial
// review.
func TestOnlyTheFiveMarkersGitHubDefinesAreStructural(t *testing.T) {
	t.Parallel()

	for _, notAnAlert := range []string{"[!INFO]", "[!DANGER]", "[!SUCCESS]", "[!QUESTION]", "[!ATTENTION]"} {
		assert.Len(t, analyze(t, "notes.md", "> "+notAnAlert+"\n> wrapped onto this line\n"), 1,
			"%s is not an alert type GitHub defines, so its newline is decorative", notAnAlert)
	}
}

// TestAsciiUpperFoldsAsciiAloneSoAMarkerCannotBeForged pins the fold the marker
// is matched under, which decides whether the exemption can be FORGED.
//
// The exemption was matched with strings.ToUpper — Unicode simple case
// mapping — and U+0131 LATIN SMALL LETTER DOTLESS I upper-cases to `I`. So
// `> [!TıP]` acquired the marker and none of the property: GitHub's alert
// syntax does not match it, the blockquote renders as a plain quotation
// carrying the literal bracketed word, and this rule fell silent on the whole
// block. An exhaustive scan of every letter below U+30000 found the dotless i
// is the only rune that reaches it, which is exactly why no fixture would have
// stumbled on it. Found by an adversarial review; the same simple-fold class
// already reproduced in yze-md-markup and yze-md-docfiles.
func TestAsciiUpperFoldsAsciiAloneSoAMarkerCannotBeForged(t *testing.T) {
	t.Parallel()

	for _, forged := range []string{"[!TıP]", "[!ıMPORTANT]", "[!WARNıNG]", "[!CAUTıON]"} {
		assert.Len(t, analyze(t, "notes.md", "> "+forged+"\n> wrapped onto this line\n"), 1,
			"%s is a lookalike GitHub renders no alert for", forged)
	}
	assert.Empty(t, analyze(t, "notes.md", "> [!tip]\n> wrapped onto this line\n"),
		"and the ASCII fold still admits the marker written in any ASCII case")
}

// TestTrimmedRemovesTheFormatsWhitespaceAndNoOtherRune pins the OTHER half of
// the path to the marker table, which hardening the fold alone left open.
//
// strings.TrimSpace removes every rune unicode.IsSpace accepts, so a marker
// padded with a non-breaking space, an ideographic space, a next line or an
// ogham space mark was deleted down to the bare marker and matched — five more
// spellings acquiring the marker and none of the property, since none of those
// is CommonMark whitespace and GitHub renders no alert for any of them. The
// commit that closed the case-fold half asserted in its own doc comment that
// every rune outside ASCII stayed foreign to the table; it did not, and only a
// case says so. Found by an adversarial review of that commit.
func TestTrimmedRemovesTheFormatsWhitespaceAndNoOtherRune(t *testing.T) {
	t.Parallel()

	for name, padded := range map[string]string{
		"a non-breaking space after":  "[!NOTE]\u00a0",
		"a non-breaking space before": "\u00a0[!NOTE]",
		"an ideographic space around": "\u3000[!NOTE]\u3000",
		"a next line after":           "[!NOTE]\u0085",
		"an ogham space mark after":   "[!NOTE]\u1680",
	} {
		assert.Len(t, analyze(t, "notes.md", "> "+padded+"\n> wrapped onto this line\n"), 1,
			"%s: the marker no longer stands alone, and GitHub renders no alert", name)
	}

	assert.Empty(t, analyze(t, "notes.md", "> [!NOTE]   \n> wrapped onto this line\n"),
		"and the format's own whitespace is still trimmed, so a real marker keeps its exemption")
	assert.Empty(t, analyze(t, "notes.md", ">\t[!NOTE]\t\n> wrapped onto this line\n"),
		"a tab included")
}

// TestReportsReadsNothingButTheBreakItIsDeciding varies the one input
// dimension this repository's corpus held constant: the TEXT of the wrapped
// line.
//
// Every case elsewhere wraps ordinary prose, so a content-keyed disjunct in the
// per-break decision was invisible to every instrument here. Adding
// `strings.Contains(string(text), "TODO") ||` to it left `make check` at exit 0
// and total coverage 100.0% — an extra disjunct inside an existing `if` adds no
// statement to reach — and left 1,028 findings of a 47-document probe corpus
// byte-identical, while being a real off switch reachable one line at a time.
// That is the exemption the package doc says must never exist: "an exemption
// available per line is not an exemption, it is the rule's off switch". The
// corpus cannot prove a negative about every possible disjunct; it can vary the
// dimension, which is what makes any disjunct keyed on it die here. Found by an
// adversarial review.
//
// Every line here is unambiguously PARAGRAPH text, and that is the point rather
// than a convenience: a line that begins a list, a heading or a blank is a
// different BLOCK to the parser, so it varies the parse instead of the text and
// would be measuring the rule's other half. Those shapes are proven where the
// parse is.
func TestReportsReadsNothingButTheBreakItIsDeciding(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"This is hard TODO", "FIXME the thing", "XXX", "HACK", "NOTE",
		"see docs/notes.md for more", "vendor/x", "node_modules", "testdata",
		"Code generated by a tool", "DO NOT EDIT", "@generated", "license", "licence",
		"[!NOTE] outside any quotation", "> not a quotation either",
		"nolint", "prettier-ignore", "yze/hardwrap", "https://example.com",
		"0", "ı", "日本語のテキスト", "a | b", "x\ty", "not a - list", "not a # heading",
	} {
		assert.Len(t, analyze(t, "notes.md", text+"\nand a second line that wraps it\n"), 1,
			"a break is reported whatever the line it ends says: %q", text)
	}
}
