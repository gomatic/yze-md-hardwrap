package hardwrap_test

// Front matter: the one region of a markdown file that is not markdown, and the
// text that merely looks like it. Both directions matter — leaving metadata in
// is a finding on every content page in a site, and taking a document out is a
// two-line opt-out from the whole rule.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFrontMatterIsNotProse pins the one region of a markdown file that is not
// markdown. Left in, a Hugo page's metadata parses as a setext heading whose
// text spans every metadata line — a finding on every content page in a site.
func TestFrontMatterIsNotProse(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"yaml": "---\ntitle: A page\ndate: 2026-01-01\ntags: [a, b]\n---\n\nA body on one line.\n",
		"toml": "+++\ntitle = \"A page\"\ndate = 2026-01-01\n+++\n\nA body on one line.\n",
	} {
		assert.Empty(t, analyze(t, "content/page.md", document), "%s front matter is metadata", name)
	}
}

// TestFindingsAfterFrontMatterNameTheAuthorsLineNumbers pins that removing the
// metadata does not move the document: a finding names the line an editor
// shows, not the line the parser saw.
func TestFindingsAfterFrontMatterNameTheAuthorsLineNumbers(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "content/page.md", "---\ntitle: A page\n---\n\nA body that is\nwrapped.\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{5}, linesOf(diags))
}

// TestAnUnclosedDelimiterIsNotFrontMatter pins the other direction. A document
// whose body opens with a thematic break is an ordinary document, and reading
// it as unterminated metadata would silence everything below the first `---`.
func TestAnUnclosedDelimiterIsNotFrontMatter(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "---\n\nA body that is\nwrapped.\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{3}, linesOf(diags), "nothing was removed, so nothing moved")
}

// TestDelimitersAroundProseAreNotFrontMatter pins the hole an adversarial
// review found, and it is the most dangerous shape this rule has: wrapping a
// whole document in `---` … `---` costs two horizontal rules in the rendered
// output and, if a delimiter pair alone were enough, deleted every finding in
// the file. It is reachable by accident too, by a page that opens with a
// decorative break and uses another one later.
func TestDelimitersAroundProseAreNotFrontMatter(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"prose between yaml delimiters": "---\n\nA body that is\nwrapped.\n\n---\n",
		"prose between toml delimiters": "+++\n\nA body that is\nwrapped.\n\n+++\n",
		"metadata and then prose":       "---\ntitle: x\n\nA body that is\nwrapped.\n---\n",
		"a list between delimiters":     "---\n- a body that is\n  wrapped\n---\n",
		"a document between delimiters": "---\n# A title\nprose that is\nwrapped\n---\n",
	} {
		assert.NotEmpty(t, analyze(t, "notes.md", document), "%s is not metadata", name)
	}
}

// TestRealFrontMatterIsStillRemoved is the other direction of that guard, in the
// shapes the fleet actually holds — including the seventeen real blocks whose
// YAML carries a blank line inside a block scalar, which every cheaper test
// would have misread.
func TestRealFrontMatterIsStillRemoved(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a mapping":           "---\ntitle: A page\nweight: 3\n---\n\nBody on one line.\n",
		"nested keys":         "---\nparams:\n  a: 1\n  b: 2\n---\n\nBody on one line.\n",
		"a list value":        "---\ntags:\n  - a\n  - b\n---\n\nBody on one line.\n",
		"a blank line inside": "---\ndescription: |\n  one\n\n  two\ntitle: x\n---\n\nBody on one line.\n",
		"an empty block":      "---\n---\n\nBody on one line.\n",
		// Twenty-one of the fleet's 1,947 YAML blocks do not DECODE, and these
		// are the two reasons: a colon inside an unquoted value, and a template
		// placeholder. Read as prose, each becomes a hard-wrapped "heading"
		// nobody wrote.
		"a value carrying a colon": "---\ndescription: Use this when X: do Y\ntitle: x\n---\n\nBody.\n",
		"a template placeholder":   "---\ntitle: \"{{t}}\"\ncreated: { { date:YYYY-MM-DD } }\n---\n\nBody.\n",
		"toml assignments":         "+++\ntitle = \"A page\"\nweight = 3\n+++\n\nBody on one line.\n",
		"a toml table":             "+++\ntitle = \"x\"\n[params]\na = 1\n+++\n\nBody on one line.\n",
	} {
		assert.Empty(t, analyze(t, "content/page.md", document), "%s is metadata", name)
	}
}

// TestADelimiterCarryingTextIsNotFrontMatter pins that the opener must be the
// delimiter ALONE, which is what Hugo requires too.
func TestADelimiterCarryingTextIsNotFrontMatter(t *testing.T) {
	t.Parallel()

	assert.Len(t, analyze(t, "notes.md", "--- title\nwrapped\n---\n"), 1)
}
