package hardwrap

// The exemption's guards, asked directly. What a document can express is proven
// through documents; what it cannot — a block the parser would never build — is
// proven here, because a guard nothing reaches is a guard nobody has checked.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yuin/goldmark/ast"
)

// TestOnlyAnHtmlCommentCanCarryTheClaim pins the shape of the exemption at the
// node level: the words are a claim only inside the one construct markdown
// renders as nothing, and a block with no text at all makes no claim.
func TestOnlyAnHtmlCommentCanCarryTheClaim(t *testing.T) {
	t.Parallel()

	const source = "<!-- @generated -->\n"
	lines := newLineIndex(source)

	assert.False(t, declaresGenerated(ast.NewParagraph(), []byte(source), lines),
		"a paragraph saying it is text a reader sees")
	assert.False(t, declaresGenerated(ast.NewHTMLBlock(ast.HTMLBlockType2), []byte(source), lines),
		"an html block with no lines carries no claim")
	assert.False(t, declaresGenerated(ast.NewHTMLBlock(ast.HTMLBlockType6), []byte(source), lines),
		"and an html block that is not a comment is visible")
}

// TestADocumentWithNoBlocksAtAllIsNotGenerated pins the empty case: nothing to
// read is not a claim of authorship.
func TestADocumentWithNoBlocksAtAllIsNotGenerated(t *testing.T) {
	t.Parallel()

	assert.False(t, isGenerated(ast.NewDocument(), nil, newLineIndex("")))
}
