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
		// The SAME question with a Windows line ending. The carriage return sits
		// between the backslashes and the newline, so a line read without
		// trimming it counts no backslashes at all and every one of these
		// becomes a finding — including the three that are visible breaks.
		crlf := analyze(t, "notes.md", "a line ending in "+run+"\r\nnext line\r\n")
		assert.Equal(t, wraps, len(crlf) == 1, "%q trailing backslashes before a CRLF", run)
	}
}

// TestAGitHubAlertMarkerIsStructuralRatherThanDecorative pins a break the parse
// alone gets wrong. `> [!NOTE]` must stand ALONE on its blockquote's first line
// for GitHub to render an alert at all — join it and the box, its icon and its
// meaning are gone — so that newline is not one that exists to fit a column.
// Found by rendering the fix through pandoc and comparing: joining it changed
// the output, which is the definition of a break a reader sees.
func TestAGitHubAlertMarkerIsStructuralRatherThanDecorative(t *testing.T) {
	t.Parallel()

	for _, marker := range []string{"[!NOTE]", "[!TIP]", "[!IMPORTANT]", "[!WARNING]", "[!CAUTION]", "[!note]"} {
		assert.Empty(t, analyze(t, "notes.md", "> "+marker+"\n> the whole warning on one line\n"),
			"%s opens an alert", marker)
	}
}

// TestAnAlertsOWNProseIsStillJudged pins the other half: the marker's newline is
// structural, and every newline BELOW it is as decorative as any other. The
// finding lands on the body, which is the line an author joins.
func TestAnAlertsOWNProseIsStillJudged(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "> [!CAUTION]\n> a warning that is\n> wrapped\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{2}, linesOf(diags))
}

// TestOnlyARealAlertMarkerIsStructural pins the narrowness. A bracketed word in
// ordinary prose is a bracketed word, a type GitHub does not define opens
// nothing, and only the FIRST line of a blockquote opens an alert — a second
// marker below it is literal body text, and exempting that one silenced a
// genuine wrap. Exempting any of them would be an opt-out anyone could type.
func TestOnlyARealAlertMarkerIsStructural(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"outside a blockquote": "[!NOTE]\nwrapped onto this line\n",
		"an undefined type":    "> [!NOTES]\n> wrapped onto this line\n",
		"carrying text":        "> [!NOTE] see this\n> wrapped onto this line\n",
		"below the first line": "> [!NOTE]\n> [!NOTE]\n> wrapped onto this line\n",
	} {
		assert.Len(t, analyze(t, "notes.md", document), 1, "%s is not an alert marker", name)
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
