package hardwrap_test

// The rule in both directions. Every case here is a shape that either IS a
// hard-wrapped block or merely looks like one, and the second half is the half
// that matters: a rule that reports a table's rows, a run of list items or a
// fenced example is one no document can be written under.

import (
	"strconv"
	"strings"
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// analyze runs the rule over one document as an unconfigured run does — the
// strict default — failing the test on a tool error.
func analyze(t *testing.T, at, source string) []goyze.Diagnostic {
	t.Helper()
	return analyzeWith(t, hardwrap.DefaultSettings(), at, source)
}

// analyzeAuthored runs the rule over one document as the ONE configuration that
// makes it lenient does: the format's own spellings for a break meant to be seen
// are left alone. The document is markdown by name, because which FILES a run
// reads is the other setting's question and is proven where that one is.
func analyzeAuthored(t *testing.T, source string) []goyze.Diagnostic {
	t.Helper()
	return analyzeWith(t, hardwrap.Settings{
		Documents: hardwrap.DefaultDocuments(),
		Breaks:    hardwrap.AuthoredBreaks,
	}, "notes.md", source)
}

// analyzeWith runs the rule over one document under a named run.
func analyzeWith(t *testing.T, settings hardwrap.Settings, at, source string) []goyze.Diagnostic {
	t.Helper()
	diags, err := hardwrap.Diagnostics(hardwrap.Path(at), hardwrap.Source(source), settings)
	require.NoError(t, err)
	return diags
}

// wrapMarker is the part of a wrap finding's message that tells it from the
// other findings this package can emit — a document it could not read, and a
// notice standing for findings past a limit. It is a fragment of the message
// rather than the whole of it so that rewording the rule's explanation does not
// rewrite every test, and it is written out here rather than imported, because
// an expectation taken from the implementation asserts only that the
// implementation is itself.
const wrapMarker = "source lines:"

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

// TestOneFindingPerBlockHoweverManyTimesItWraps pins the unit of the report. A
// paragraph wrapped over twenty lines is one mistake and one edit; reporting it
// nineteen times would make the ratchet move by nineteen for one fix.
func TestOneFindingPerBlockHoweverManyTimesItWraps(t *testing.T) {
	t.Parallel()

	diags := analyze(t, "notes.md", "one\ntwo\nthree\nfour\nfive\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{1}, linesOf(diags), "positioned at the first reported break")
}

// TestEveryWrappingConstructIsNamedByItsOwnKind pins that the finding tells an
// author WHICH construct to join, and that each is found in the first place.
func TestEveryWrappingConstructIsNamedByItsOwnKind(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"paragraph": "wrapped over\ntwo lines\n",
		"list item": "- an item that is\n  wrapped\n",
		"heading":   "a setext title that is\nwrapped\n=======================\n",
	} {
		diags := analyze(t, "notes.md", document)
		require.Len(t, diags, 1, "%s wraps", name)
		assert.Contains(t, diags[0].Message, name)
	}
}

// TestADefinitionListIsOrdinaryProse pins a reversal, and the reason for it.
//
// The parser used to read definition lists, so that a `Term` / `: value` pair
// was two blocks rather than one wrapped paragraph. Under that reading EVERY
// line above a `:` is a term of its own — so appending one `: note` line to
// twenty lines of hard-wrapped prose made twenty one-line terms, and the whole
// passage went unreported. A second adversarial review found it, the fleet was
// measured at ZERO definition lists, and the extension was removed: the
// exemption protected nothing and cost a bypass anyone could type. GitHub
// renders this text as one paragraph in the first place.
func TestADefinitionListIsOrdinaryProse(t *testing.T) {
	t.Parallel()

	assert.Len(t, analyze(t, "notes.md", "a term that is\nwrapped\n: its definition\n"), 1)
	assert.Len(t, analyze(t, "notes.md", strings.Repeat("a line of ordinary prose\n", 20)+": note\n"), 1,
		"and a trailing definition marker does not turn a passage into terms")
	assert.Empty(t, analyze(t, "notes.md", "Term\n\n: its definition on one line\n"),
		"while prose written one line per block is still silent, marker or no marker")
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

// TestAFindingNamesHowManySourceLinesTheBlockSpans pins the number in the
// message. It is the size of the edit being asked for, and it is the whole of
// what the message tells a reader beyond the rule itself.
func TestAFindingNamesHowManySourceLinesTheBlockSpans(t *testing.T) {
	t.Parallel()

	for spans, document := range map[int]string{
		2: "one\ntwo\n",
		3: "one\ntwo\nthree\n",
		5: "one\ntwo\nthree\nfour\nfive\n",
	} {
		diags := analyze(t, "notes.md", document)

		require.Len(t, diags, 1)
		assert.Contains(t, diags[0].Message, "spans "+strconv.Itoa(spans)+" source lines",
			"a block occupying %d lines is asked to become one", spans)
	}
}

// TestNoFindingNamesAWayToSilenceTheRule pins the message discipline, and it is
// the reason this rule can be gated at all.
//
// A diagnostic is read at the exact moment its reader is looking for the
// cheapest way to make the gate green, and much of what reads it is an agent
// applying whatever it finds mechanically to every line it touches. A message
// naming a spelling this rule leaves alone would be a bypass tutorial delivered
// by the tool itself: one that turns a rule about how prose is written into a
// rule about how line endings are spelled, forever, with the gate green.
//
// It covers EVERY message this package can emit rather than the wrap finding
// alone, because a limit notice and a read failure are read in the same moment
// and by the same reader.
func TestNoFindingNamesAWayToSilenceTheRule(t *testing.T) {
	t.Parallel()

	for _, message := range everyMessage(t) {
		for name, forbidden := range map[string]string{
			"a backslash":            `\`,
			"a run of spaces":        "  ",
			"a markup tag":           "<br",
			"the word for a space":   "space",
			"the setting's variable": "YZE_",
			"the setting's value":    "authored",
			"the word escape":        "escape",
		} {
			assert.NotContains(t, message, forbidden, "%s is a way to silence this rule, and %q names it",
				name, message)
		}
	}
}

// everyMessage is each distinct message this package emits: a wrapped block, a
// document past its own limit, a run past the run's limit, and a file the gate
// could not read — the last in both of its shapes, a path the walk could not
// enter and a document the reader refused.
func everyMessage(t *testing.T) []string {
	t.Helper()
	contents, files := crowded(30, 500)
	contents["huge.md"] = strings.Repeat("wrapped one\nwrapped two\n\n", 1200)
	messages := map[string]bool{}
	for _, report := range []goyze.Report{
		hardwrap.Report(reader(contents), []string{"huge.md", "d0.md", "locked.md"}, nil, hardwrap.DefaultSettings()),
		hardwrap.Report(reader(contents), files, []string{"locked"}, hardwrap.DefaultSettings()),
	} {
		for _, diag := range report.Diagnostics {
			messages[diag.Message] = true
		}
	}
	found := make([]string, 0, len(messages))
	for message := range messages {
		found = append(found, message)
	}
	require.Len(t, found, 5, "every distinct message this package emits")
	return found
}
