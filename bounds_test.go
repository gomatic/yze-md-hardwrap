package hardwrap_test

// What this rule refuses to read, and how much of what it finds it carries.
// Every bound here is proven on BOTH sides, because one either side of a limit
// is what tells a `>` from a `>=` — and because a bound that turns away a
// document it was sized to accept is a silence.

import (
	"strings"
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// TestOnlyConfiguredDocumentsAreRead pins that a path outside the run's
// document set yields nothing and no error: it is not this rule's business,
// which is a different answer from "it is clean".
func TestOnlyConfiguredDocumentsAreRead(t *testing.T) {
	t.Parallel()

	const wrapped = "a paragraph that is\nwrapped\n"

	assert.Empty(t, analyze(t, "notes.txt", wrapped), "plain text is opt-in")
	assert.Empty(t, analyze(t, "main.go", wrapped), "code is not prose")
	assert.Len(t, analyze(t, "docs/NOTES.MD", wrapped), 1, "an extension is matched however it is spelled")
	assert.Len(t, analyze(t, "docs/notes.markdown", wrapped), 1)
}

// TestAnOptedInDocumentIsParsedAsMarkdown pins the deliberate decision behind
// the opt-in: there is no plain-text mode. A file the run was told to read is
// read as markdown, which is what makes the answer exact rather than a guess
// about what a line ending means in an unknown format.
func TestAnOptedInDocumentIsParsedAsMarkdown(t *testing.T) {
	t.Parallel()

	settings, err := hardwrap.ConfiguredSettings(environment(map[string]string{documentsVariable: ".txt"}))
	require.NoError(t, err)

	diags, err := hardwrap.Diagnostics("notes.txt", "a paragraph that is\nwrapped\n\n```\nnot\nprose\n```\n", settings)

	require.NoError(t, err)
	require.Len(t, diags, 1, "its prose is judged and its fenced block is not")
	assert.Equal(t, []int{1}, linesOf(diags))
}

// TestADocumentTooLargeToReadIsRefused pins the bound, and pins it as the
// SHARED sentinel: a caller matching a local copy would match only for as long
// as the two happened to carry the same text.
func TestADocumentTooLargeToReadIsRefused(t *testing.T) {
	t.Parallel()

	over := strings.Repeat("x", int(hardwrap.SizeLimit)+1)

	diags, err := hardwrap.Diagnostics("huge.md", hardwrap.Source(over), hardwrap.DefaultSettings())

	assert.Empty(t, diags, "a tool failure yields no findings")
	require.ErrorIs(t, err, hardwrap.ErrTooLarge)
	assert.ErrorIs(t, err, goyze.ErrTooLarge, "the shared sentinel, not a second one beside it")
}

// TestADocumentExactlyAtTheLimitIsStillRead pins the other side of the bound.
// One byte either side is what tells a `>` from a `>=`, and a limit that turned
// away the document AT it would refuse a file it was sized to accept.
func TestADocumentExactlyAtTheLimitIsStillRead(t *testing.T) {
	t.Parallel()

	const head = "a paragraph that is\nwrapped\n"
	exact := head + strings.Repeat("x", int(hardwrap.SizeLimit)-len(head))

	diags, err := hardwrap.Diagnostics("big.md", hardwrap.Source(exact), hardwrap.DefaultSettings())

	require.NoError(t, err)
	assert.Len(t, diags, 1, "the document at the limit is read and judged")
}

// TestADocumentNestedPastTheBoundIsRefusedRatherThanParsed pins the bound that
// the size limit does not provide: a parser opens one block parser per container
// per line, so cost grows with the SQUARE of nesting depth while the byte count
// barely moves. A document written to be slow is refused as a finding — never
// passed over, and never parsed for hours.
func TestADocumentNestedPastTheBoundIsRefusedRatherThanParsed(t *testing.T) {
	t.Parallel()

	deep := strings.Repeat(">", 65) + " a\n" + strings.Repeat(">", 65) + " b\n"

	diags, err := hardwrap.Diagnostics("deep.md", hardwrap.Source(deep), hardwrap.DefaultSettings())

	assert.Empty(t, diags, "a tool failure yields no findings")
	assert.ErrorIs(t, err, hardwrap.ErrTooDeep)
}

// TestADocumentAtTheNestingBoundIsStillJudged pins the other side of it. The
// bound is twenty times the deepest nesting in the fleet, so everything an
// author writes is read — including the quotation nested at the bound itself.
func TestADocumentAtTheNestingBoundIsStillJudged(t *testing.T) {
	t.Parallel()

	marker := strings.Repeat(">", 64)
	deep := marker + " a quotation that is\n" + marker + " wrapped\n"

	diags, err := hardwrap.Diagnostics("deep.md", hardwrap.Source(deep), hardwrap.DefaultSettings())

	require.NoError(t, err)
	assert.Len(t, diags, 1)
	assert.Empty(t, analyze(t, "notes.md", "> > > a quotation on one line\n"), "and ordinary nesting is silent")
}

// TestADocumentThatIsNotTextIsRefused pins that a binary blob is a tool failure
// rather than a clean pass. A markdown parser given arbitrary bytes still
// produces blocks, so the findings would be invented from whatever byte
// happened to end a line.
func TestADocumentThatIsNotTextIsRefused(t *testing.T) {
	t.Parallel()

	diags, err := hardwrap.Diagnostics(
		"blob.md",
		hardwrap.Source([]byte{0xff, 0xfe, 0x00}),
		hardwrap.DefaultSettings(),
	)

	assert.Empty(t, diags)
	assert.ErrorIs(t, err, hardwrap.ErrNotText)
}

// TestADocumentPastItsOwnLimitIsTruncatedAndCounted pins that a pathological
// document costs a bounded report, and that the count it truncates is never
// silently lost.
func TestADocumentPastItsOwnLimitIsTruncatedAndCounted(t *testing.T) {
	t.Parallel()

	document := strings.Repeat("wrapped one\nwrapped two\n\n", 1200)

	diags := analyze(t, "huge.md", document)

	require.Len(t, diags, 1001, "the limit, plus the notice that stands for the rest")
	assert.Contains(t, diags[1000].Message, "1200 hard-wrapped blocks in this document")
	assert.Contains(t, diags[1000].Message, "of which 1000 are reported")
}

// TestADocumentExactlyAtItsLimitCarriesNoTruncationNotice pins that boundary
// too. The notice stands for findings that were DROPPED, so a document whose
// findings all fit must not carry one — one either side is what tells a `>`
// from a `>=`.
func TestADocumentExactlyAtItsLimitCarriesNoTruncationNotice(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "huge.md", strings.Repeat("wrapped one\nwrapped two\n\n", 1000))

	require.Len(t, diags, 1000)
	for _, diag := range diags {
		assert.Contains(t, diag.Message, wrapMarker, "no notice among them")
	}
}

// TestEveryKindOfContainerCountsTowardsTheNestingBound pins the bound against
// the containers a second adversarial review found it was not counting. A
// parser opens one block parser per container per line, so the cost is the
// SQUARE of the depth however the depth is spelled: `- ` repeated fifty thousand
// times is one 100 KB line that took sixteen seconds, and a document of steadily
// indented list items took eighteen. Both are now refused in hundredths of a
// second, as findings rather than silences.
func TestEveryKindOfContainerCountsTowardsTheNestingBound(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"blockquote markers":     strings.Repeat(">", 65) + " a\n" + strings.Repeat(">", 65) + " b\n",
		"bullet markers":         strings.Repeat("- ", 65) + "x\n",
		"star markers":           strings.Repeat("* ", 65) + "x\n",
		"ordered markers":        strings.Repeat("1. ", 65) + "x\n",
		"markers across lines":   nestedList(65),
		"markers and quotations": strings.Repeat("> ", 33) + strings.Repeat("- ", 33) + "x\n",
		// A tab is four columns of indentation, which is what CommonMark counts
		// it as — so a tab-indented list reaches the bound in a quarter of the
		// lines, and a counter that read a tab as one column would miss it.
		"markers behind tabs": strings.Repeat("\t", 32) + "- item\n",
	} {
		diags, err := hardwrap.Diagnostics("deep.md", hardwrap.Source(document), hardwrap.DefaultSettings())

		assert.Empty(t, diags, "%s: a tool failure yields no findings", name)
		assert.ErrorIs(t, err, hardwrap.ErrTooDeep, "%s open containers this rule will not parse", name)
	}
}

// nestedList is a list nested to depth levels, one item per line, the way a
// deeply nested list is really written.
func nestedList(depth int) string {
	document := strings.Builder{}
	for level := range depth {
		// The builder never fails; its error exists for io.Writer's sake.
		_, _ = document.WriteString(strings.Repeat("  ", level) + "- item\n")
	}
	return document.String()
}

// TestNothingAnybodyWritesReachesTheNestingBound is the other direction, and the
// one that decides whether the bound is usable. The deepest line in any markdown
// file on disk scores seventeen and the deepest first-party line scores eight, so
// every one of these is judged rather than refused — including the shapes that
// LOOK deep to a counter: an indented code block, and a deeply indented sample
// inside a fence, neither of which opens a container at all.
func TestNothingAnybodyWritesReachesTheNestingBound(t *testing.T) {
	t.Parallel()

	const wrapped = "\na paragraph that is\nwrapped\n"

	for name, document := range map[string]string{
		"a list nested eight deep":     nestedList(8) + wrapped,
		"a quotation nested three":     "> > > quoted on one line\n" + wrapped,
		"an indented code block":       "text\n\n" + strings.Repeat(" ", 44) + "deep code\n" + wrapped,
		"a deeply indented fence":      "```yaml\n" + strings.Repeat(" ", 40) + "- name: x\n```\n" + wrapped,
		"a list inside a quotation":    "> - a\n>   - b\n>     - c\n" + wrapped,
		"a paragraph starting with a-": "- a dash mid sentence - like this\n" + wrapped,
		"a tab-indented list":          "- a\n\t- b\n" + wrapped,
	} {
		diags, err := hardwrap.Diagnostics("notes.md", hardwrap.Source(document), hardwrap.DefaultSettings())

		require.NoError(t, err, "%s is a document somebody wrote", name)
		assert.Len(t, diags, 1, "%s is judged, and its wrapped paragraph found", name)
	}
}

// TestAMarkerAtTheEndOfALineIsStillOneMarker pins the degenerate lines the
// depth counter has to survive. A list marker with nothing after it is an empty
// list item — a real document, however odd — and reading a marker and a space
// off a line holding only the marker panicked. The fuzz target found it, which
// is the only place a one-character document was going to come from.
func TestAMarkerAtTheEndOfALineIsStillOneMarker(t *testing.T) {
	t.Parallel()

	for _, document := range []string{"*", "-", "+", "1.", "1)", "0", ">", "- ", "-\n- a\n", "1.\n", "> -"} {
		diags, err := hardwrap.Diagnostics("notes.md", hardwrap.Source(document), hardwrap.DefaultSettings())

		require.NoError(t, err, "%q is a document, however degenerate", document)
		assert.Empty(t, diags, "%q occupies one line per block", document)
	}
}

// TestAPathTheRunDoesNotReadIsRefusedBeforeItsBytesAre pins the ORDER of the
// two gates, which is a contract and not an arrangement of independent checks.
//
// [hardwrap.Diagnostics] promises that a path the run does not read yields no
// findings AND NO ERROR — "it is not this rule's business, which is a different
// answer from it is clean". As shipped it asked the content questions first, so
// `main.go` came back ErrTooLarge at 8 MiB + 1 bytes, ErrNotText for two
// non-UTF-8 bytes and ErrTooDeep for deeply nested containers: a tool failure
// raised about a file the rule had already decided to say nothing about, and
// one an author cannot act on because the file is not a document. The test
// named for the contract only ever passed inputs that clear every guard, so it
// could not see the order in either direction. Found by an adversarial review.
func TestAPathTheRunDoesNotReadIsRefusedBeforeItsBytesAre(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"past the size bound":   strings.Repeat("a", int(hardwrap.SizeLimit)+1),
		"not text at all":       "\xff\xfe",
		"nested past the bound": strings.Repeat("> ", 200) + "x",
	} {
		diags, err := hardwrap.Diagnostics("main.go", hardwrap.Source(source), hardwrap.DefaultSettings())

		require.NoError(t, err, "%s: a path outside the document set is not this rule's business", name)
		assert.Empty(t, diags, name)
	}
}

// TestADocumentTheRunDOESReadStillMeetsEveryGuard is the in-scope sibling of
// the case above, so the silence there is never mistaken for the guards having
// been removed.
func TestADocumentTheRunDOESReadStillMeetsEveryGuard(t *testing.T) {
	t.Parallel()

	for name, source := range map[string]string{
		"past the size bound":   strings.Repeat("a", int(hardwrap.SizeLimit)+1),
		"not text at all":       "\xff\xfe",
		"nested past the bound": strings.Repeat("> ", 200) + "x",
	} {
		_, err := hardwrap.Diagnostics("notes.md", hardwrap.Source(source), hardwrap.DefaultSettings())

		require.Error(t, err, "%s: a document this run reads is still held to every bound", name)
	}
}
