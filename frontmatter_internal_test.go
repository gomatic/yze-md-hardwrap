package hardwrap

// Splitting a document into lines, and telling a metadata delimiter from a
// thematic break. Both are proven here because the shapes that matter — a final
// line with no newline after it, a delimiter that never closes — are edges a
// document cannot express twice over.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAFinalLineWithNoNewlineIsStillALine pins the boundary a split gets wrong.
// A file whose last line is unterminated is ordinary; losing it would drop the
// last paragraph of every such document.
func TestAFinalLineWithNoNewlineIsStillALine(t *testing.T) {
	t.Parallel()

	first, rest, ok := nextLine("only line")

	require.True(t, ok)
	assert.Equal(t, line("only line"), first)
	assert.Equal(t, Source(""), rest)

	_, _, more := nextLine(rest)
	assert.False(t, more, "and the document has then run out")
}

// TestAnEmptyLineIsALine pins that a blank line is content, not the end of the
// document: a run of them separates blocks, and treating one as exhaustion
// would stop reading at the first paragraph break.
func TestAnEmptyLineIsALine(t *testing.T) {
	t.Parallel()

	first, rest, ok := nextLine("\nbody")

	require.True(t, ok)
	assert.Equal(t, line(""), first)
	assert.Equal(t, Source("body"), rest)
}

// TestOnlyADelimiterAloneOpensFrontMatter pins what may open a metadata block.
// The delimiter is the whole line — trailing whitespace and a carriage return
// are an editor's, not an author's — and anything else is prose.
func TestOnlyADelimiterAloneOpensFrontMatter(t *testing.T) {
	t.Parallel()

	for _, opener := range []line{"---", "+++", "--- ", "---\r", "+++\t"} {
		assert.True(t, opensFrontMatter(opener), "%q is a delimiter alone", opener)
	}
	for _, prose := range []line{"", "----", "--- title", "-- -", "a---", "+++title"} {
		assert.False(t, opensFrontMatter(prose), "%q is not", prose)
	}
}

// TestAnUnterminatedDelimiterRemovesNothing pins the refusal: with no closing
// delimiter the document is returned whole, so a page opening with a thematic
// break keeps every block below it.
func TestAnUnterminatedDelimiterRemovesNothing(t *testing.T) {
	t.Parallel()

	const document Source = "---\ntitle: never closed\n\nbody"

	body, removed := withoutFrontMatter(document)

	assert.Equal(t, document, body)
	assert.Equal(t, lineOffset(0), removed)
}

// TestTheDelimiterMustCloseWithITSOWNSPELLING pins that the two spellings are
// not interchangeable: a YAML block is closed by `---`, and a `+++` inside it
// is metadata rather than its end.
func TestTheDelimiterMustCloseWithITSOWNSPELLING(t *testing.T) {
	t.Parallel()

	body, removed := withoutFrontMatter("---\nsep: +++\n---\nbody\n")

	assert.Equal(t, Source("body\n"), body)
	assert.Equal(t, lineOffset(3), removed)
}
