package hardwrap

// The two helpers whose contracts are TOTAL — they answer for inputs a parse
// cannot produce — proven here rather than through a document, because a
// document cannot reach them and a guard nothing reaches is a guard nobody has
// checked.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/ast"
)

// TestALineNumberIsAlwaysNavigable pins the contract of the index: whatever
// offset it is asked about, it answers with a line an editor can open. Line
// zero is not one.
func TestALineNumberIsAlwaysNavigable(t *testing.T) {
	t.Parallel()

	lines := newLineIndex("one\ntwo\nthree")

	assert.Equal(t, lineNumber(1), lines.of(0), "the first byte is on the first line")
	assert.Equal(t, lineNumber(1), lines.of(3), "the newline belongs to the line it ends")
	assert.Equal(t, lineNumber(2), lines.of(4))
	assert.Equal(t, lineNumber(3), lines.of(10))
	assert.Equal(t, lineNumber(3), lines.of(9_999), "past the end is still the last line")
	assert.Equal(t, lineNumber(1), lines.of(-1), "and before the beginning is still the first")
}

// TestAnEmptyDocumentStillHasAFirstLine pins the degenerate index rather than
// leaving it to a caller to discover.
func TestAnEmptyDocumentStillHasAFirstLine(t *testing.T) {
	t.Parallel()

	assert.Equal(t, lineNumber(1), newLineIndex("").of(0))
}

// TestAnUnnamedBlockIsStillDescribed pins the fallback. Naming a construct is
// how a finding tells an author what to join, so a node this table has no entry
// for — its own kind or its parent's — is described rather than named nothing
// at all.
func TestAnUnnamedBlockIsStillDescribed(t *testing.T) {
	t.Parallel()

	assert.Equal(t, blockKind("block"), blockName(ast.NewTextBlock()), "no entry, and no parent to ask")
	assert.Equal(t, blockKind("block"), blockName(ast.NewDocument()), "a kind with no entry of its own")
}

// TestABlockIsNamedByItsParentWhenItHasNoNameOfItsOwn pins the other half: the
// anonymous text block a tight list item holds its prose in is named for the
// construct an author would recognise.
func TestABlockIsNamedByItsParentWhenItHasNoNameOfItsOwn(t *testing.T) {
	t.Parallel()

	item := ast.NewListItem(0)
	text := ast.NewTextBlock()
	item.AppendChild(item, text)

	assert.Equal(t, blockKind("list item"), blockName(text))
}
