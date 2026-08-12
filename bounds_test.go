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

	docs := hardwrap.ConfiguredDocuments(func(string) string { return ".txt" })

	diags, err := hardwrap.Diagnostics("notes.txt", "a paragraph that is\nwrapped\n\n```\nnot\nprose\n```\n", docs)

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

	diags, err := hardwrap.Diagnostics("huge.md", hardwrap.Source(over), hardwrap.DefaultDocuments())

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

	diags, err := hardwrap.Diagnostics("big.md", hardwrap.Source(exact), hardwrap.DefaultDocuments())

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

	diags, err := hardwrap.Diagnostics("deep.md", hardwrap.Source(deep), hardwrap.DefaultDocuments())

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

	diags, err := hardwrap.Diagnostics("deep.md", hardwrap.Source(deep), hardwrap.DefaultDocuments())

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
		hardwrap.DefaultDocuments(),
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
		assert.Contains(t, diag.Message, "is hard-wrapped", "no notice among them")
	}
}
