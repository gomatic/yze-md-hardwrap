package hardwrap_test

// The rule in both directions. Every case here is a shape that either IS a
// hard-wrapped block or merely looks like one, and the second half is the half
// that matters: a rule that reports a table's rows, a run of list items or a
// fenced example is one no document can be written under.

import (
	"strings"
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// analyze runs the rule over one document, failing the test on a tool error.
func analyze(t *testing.T, at, source string) []goyze.Diagnostic {
	t.Helper()
	diags, err := hardwrap.Diagnostics(hardwrap.Path(at), hardwrap.Source(source), hardwrap.DefaultDocuments())
	require.NoError(t, err)
	return diags
}

// linesOf is where a document's findings are, which is the half of a finding an
// author navigates by.
func linesOf(diags []goyze.Diagnostic) []int {
	found := make([]int, 0, len(diags))
	for _, diag := range diags {
		found = append(found, diag.Line)
	}
	return found
}

// TestAWrappedParagraphIsReportedAtTheLineThatWraps pins the rule itself: the
// newline between these two lines is one no renderer shows, so it exists only
// to fit a column.
func TestAWrappedParagraphIsReportedAtTheLineThatWraps(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "A paragraph that is\nwrapped here.\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{1}, linesOf(diags), "the line whose ending newline vanishes")
	assert.Equal(t, hardwrap.Rule, diags[0].Rule)
	assert.Equal(t, goyze.SeverityError, diags[0].Severity)
	assert.Contains(t, diags[0].Message, "paragraph")
}

// TestProseWrittenOneLinePerBlockIsSilent is the control. However long a line
// is, a block that occupies one of them renders exactly as it is written.
func TestProseWrittenOneLinePerBlockIsSilent(t *testing.T) {
	t.Parallel()

	document := "# A heading\n\n" + strings.Repeat("a long sentence that keeps going, ", 40) + "\n\n" +
		"- one list item on one line\n- another\n\n> a quotation on one line\n"

	assert.Empty(t, analyze(t, "notes.md", document), "line length is not this rule's business")
}

// TestAnExplicitHardBreakIsNeverReported pins the escape hatch the format
// already provides. A break the author asked to be SEEN is visible in the
// output, so it is not a break that exists to fit a column — and a rule without
// this exemption would have no way to write a visible line break at all.
func TestAnExplicitHardBreakIsNeverReported(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a trailing backslash": "first line\\\nsecond line\n",
		"two trailing spaces":  "first line  \nsecond line\n",
	} {
		assert.Empty(t, analyze(t, "notes.md", document), "%s is a visible break", name)
	}
}

// TestOnlyTheInvisibleBreaksOfAMixedBlockAreReported pins that the two kinds of
// break are judged separately within one paragraph: the visible one is left
// alone and the invisible one below it is still found.
func TestOnlyTheInvisibleBreaksOfAMixedBlockAreReported(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "one\\\ntwo\nthree\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{2}, linesOf(diags), "the backslash line is exempt, the bare newline below it is not")
}

// TestABackslashRunIsResolvedAsEscapesBeforeTheBreak pins the boundary a
// scanner gets wrong and goldmark gets wrong differently. CommonMark resolves
// the escapes first: an EVEN run is literal backslashes and the newline is
// still invisible, while an ODD run leaves a lone backslash against the newline
// and the break is one a reader sees.
//
// goldmark decides this from the last two characters, so it calls three
// backslashes escaped text; trusting it would report the one shape whose whole
// purpose is to make the break visible. The fuzz target found it on `0\\\`.
func TestABackslashRunIsResolvedAsEscapesBeforeTheBreak(t *testing.T) {
	t.Parallel()

	for run, wraps := range map[string]bool{
		`\`:      false,
		`\\`:     true,
		`\\\`:    false,
		`\\\\`:   true,
		`\\\\\`:  false,
		`\\\\\\`: true,
	} {
		diags := analyze(t, "notes.md", "a line ending in "+run+"\nnext line\n")
		assert.Equal(t, wraps, len(diags) == 1, "%q trailing backslashes: wraps=%v", run, wraps)
	}
}

// TestOneFindingPerBlockHoweverManyTimesItWraps pins the unit of the report. A
// paragraph wrapped over twenty lines is one mistake and one edit; reporting it
// nineteen times would make the ratchet move by nineteen for one fix.
func TestOneFindingPerBlockHoweverManyTimesItWraps(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "one\ntwo\nthree\nfour\nfive\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{1}, linesOf(diags), "positioned at the first invisible break")
}

// TestEveryWrappingConstructIsNamedByItsOwnKind pins that the finding tells an
// author WHICH construct to join, and that each is found in the first place.
func TestEveryWrappingConstructIsNamedByItsOwnKind(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"paragraph":  "wrapped over\ntwo lines\n",
		"list item":  "- an item that is\n  wrapped\n",
		"heading":    "a setext title that is\nwrapped\n=======================\n",
		"definition": "Term\n: a definition that is\n  wrapped\n",
	} {
		diags := analyze(t, "notes.md", document)
		require.Len(t, diags, 1, "%s wraps", name)
		assert.Contains(t, diags[0].Message, name)
	}
}

// TestADefinitionTermCannotWrap pins why no finding names one: each line above
// a `:` is a term of its own, so two lines are two terms rather than one term
// wrapped.
func TestADefinitionTermCannotWrap(t *testing.T) {
	t.Parallel()

	assert.Empty(t, analyze(t, "notes.md", "a term that is\nwrapped\n: its definition\n"))
}

// TestAWrappedBlockquoteIsReportedOnce pins the containment of the walk: a
// blockquote holds the paragraph that wraps, and attributing the break to both
// would report one wrap twice.
func TestAWrappedBlockquoteIsReportedOnce(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a quotation":        "> quoted line one\n> quoted line two\n",
		"a nested quotation": "> > deep line one\n> > deep line two\n",
		"a lazy continuation": "> quoted line one\n" +
			"continued without its marker\n",
	} {
		assert.Len(t, analyze(t, "notes.md", document), 1, "%s wraps once", name)
	}
}

// TestConstructsThatMerelyLOOKLikeWrappedProseAreSilent is the half of the rule
// that decides whether it is usable. Each of these is several consecutive lines
// of text that the parser knows is not one paragraph — and a scanner would have
// to re-decide every one of them, differently.
func TestConstructsThatMerelyLOOKLikeWrappedProseAreSilent(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a table":                      "| a | b |\n| - | - |\n| 1 | 2 |\n| 3 | 4 |\n",
		"consecutive list items":       "- one\n- two\n- three\n",
		"a numbered list":              "1. one\n2. two\n3. three\n",
		"link reference definitions":   "[a]: http://a\n[b]: http://b\n[c]: http://c\n",
		"footnote definitions":         "text[^1]\n\n[^1]: first note\n[^2]: second note\n",
		"a definition list":            "Term\n: its definition\n\nOther\n: its definition\n",
		"a fenced code block":          "```go\nfunc main() {\n}\n```\n",
		"a tilde fence":                "~~~\nline one\nline two\n~~~\n",
		"nested fences":                "````\n```\nline one\nline two\n```\n````\n",
		"an indented code block":       "    line one\n    line two\n",
		"an html block":                "<div>\n<span>x</span>\n</div>\n",
		"consecutive atx headings":     "# One\n## Two\n### Three\n",
		"a setext underline":           "A title\n=======\n\nbody\n",
		"a thematic break":             "one line\n\n---\n\nanother line\n",
		"a task list":                  "- [ ] one\n- [x] two\n",
		"a code block inside an item":  "- item\n\n  ```\n  one\n  two\n  ```\n",
		"a table with escaped pipes":   "| a | b |\n| - | - |\n| x \\| y | z |\n",
		"an html comment across lines": "<!--\na note\nacross lines\n-->\n",
	} {
		assert.Empty(t, analyze(t, "notes.md", document), "%s is not a wrapped block", name)
	}
}

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

// TestADelimiterCarryingTextIsNotFrontMatter pins that the opener must be the
// delimiter ALONE, which is what Hugo requires too.
func TestADelimiterCarryingTextIsNotFrontMatter(t *testing.T) {
	t.Parallel()

	assert.Len(t, analyze(t, "notes.md", "--- title\nwrapped\n---\n"), 1)
}

// TestLineEndingsAndAByteOrderMarkDoNotMoveAFinding pins the two invisible
// differences an editor introduces. A carriage return is part of the line
// ending, not of the prose, and a byte order mark is not the first character of
// the first paragraph.
func TestLineEndingsAndAByteOrderMarkDoNotMoveAFinding(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"crlf":                    "a paragraph that is\r\nwrapped\r\n",
		"a byte order mark":       "\ufeffa paragraph that is\nwrapped\n",
		"crlf after front matter": "---\r\ntitle: x\r\n---\r\n\r\nnope\r\n",
	} {
		diags := analyze(t, "notes.md", document)
		if name == "crlf after front matter" {
			assert.Empty(t, diags, "%s: metadata is still metadata", name)
			continue
		}
		require.Len(t, diags, 1, "%s wraps", name)
		assert.Equal(t, []int{1}, linesOf(diags), "%s does not move the line", name)
	}
}

// TestACrlfHardBreakIsStillAHardBreak pins that the escape hatch survives a
// Windows line ending, which is the shape most likely to lose it.
func TestACrlfHardBreakIsStillAHardBreak(t *testing.T) {
	t.Parallel()

	assert.Empty(t, analyze(t, "notes.md", "first line\\\r\nsecond line\r\n"))
	assert.Empty(t, analyze(t, "notes.md", "first line  \r\nsecond line\r\n"))
}

// TestAGeneratedDocumentIsOutOfScopeEntirely pins the ratified scope: the rule
// is about hand-maintained prose. Reporting a generated file tells an author to
// reflow a document the next run overwrites.
func TestAGeneratedDocumentIsOutOfScopeEntirely(t *testing.T) {
	t.Parallel()

	for _, claim := range []string{
		"<!-- Code generated by tool. DO NOT EDIT. -->",
		"<!-- Code generated by gomarkdoc. DO NOT EDIT -->",
		"<!-- @generated -->",
		"<!--\n@generated\n-->",
	} {
		assert.Empty(
			t,
			analyze(t, "api.md", claim+"\n\nwrapped one\nwrapped two\n"),
			"%s is a generator's claim",
			claim,
		)
	}
}

// TestOnlyAnInvisibleClaimExemptsADocument pins the attack surface of that
// exemption. Every shape here puts the same words where a READER sees them, and
// accepting one would be a one-line, audit-trail-free opt-out from every
// finding in the file.
func TestOnlyAnInvisibleClaimExemptsADocument(t *testing.T) {
	t.Parallel()

	for name, header := range map[string]string{
		"prose":            "@generated",
		"a fenced example": "```\n<!-- @generated -->\n```",
		"an indented one":  "    <!-- @generated -->",
		"a list item":      "- <!-- @generated -->",
		"a quotation":      "> <!-- @generated -->",
		"a heading":        "# Code generated by hand. DO NOT EDIT.",
	} {
		assert.NotEmpty(t, analyze(t, "notes.md", header+"\n\nwrapped one\nwrapped two\n"),
			"%s is visible, so it is not a generator's claim", name)
	}
}

// TestAClaimBelowTheHeaderIsANoteAboutAPassage pins that the exemption is a
// statement about the FILE. A comment further down is a note beside a passage,
// and a document about the convention is still the hand-written document it is.
func TestAClaimBelowTheHeaderIsANoteAboutAPassage(t *testing.T) {
	t.Parallel()

	document := "# Title\n\nline\n\nline\n\nline\n\n<!-- @generated -->\n\nwrapped one\nwrapped two\n"

	assert.NotEmpty(t, analyze(t, "notes.md", document))
}

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
