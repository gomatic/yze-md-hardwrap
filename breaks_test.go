package hardwrap_test

// How a line ending is judged, and by which run.
//
// The half of this rule that decides whether it can be gated at all is here:
// every spelling of a line ending is a finding by default, and the one
// configuration that changes that is a property of the RUN. A rule with a
// per-line escape is a rule with an off switch, and an off switch a writer can
// reach one line at a time is not an exemption — it is the end of the rule.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEverySpellingOfALineEndingIsReportedByDefault pins the rule an
// unconfigured run applies, which is the whole standard: one physical line per
// block, whatever the source did to end its lines.
//
// Every spelling here renders as a visible break, and every one of them was
// once exempt. That exemption was the rule's off switch: it is available per
// line, it needs no configuration, it leaves no trace in review, and appending
// two spaces to every wrapped line keeps a document hard-wrapped forever with
// the gate green.
func TestEverySpellingOfALineEndingIsReportedByDefault(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a trailing backslash":    "first line\\\nsecond line\n",
		"two trailing spaces":     "first line  \nsecond line\n",
		"three trailing spaces":   "first line   \nsecond line\n",
		"a trailing tag":          "first line<br>\nsecond line\n",
		"a trailing closed tag":   "first line<br/>\nsecond line\n",
		"a trailing spaced tag":   "first line<br />\nsecond line\n",
		"a backslash before crlf": "first line\\\r\nsecond line\r\n",
		"two spaces before crlf":  "first line  \r\nsecond line\r\n",
		"a run of backslashes":    "first line\\\\\\\nsecond line\n",
	} {
		diags := analyze(t, "notes.md", document)
		require.Len(t, diags, 1, "%s spans two lines and the standard is one", name)
		assert.Equal(t, []int{1}, linesOf(diags), "%s is reported at the line that wraps", name)
	}
}

// TestTheConfiguredRunLeavesTheFormatsOwnBreaksAlone pins the ONE way a run is
// made lenient, and pins that it is the run's configuration that does it rather
// than anything in the document.
func TestTheConfiguredRunLeavesTheFormatsOwnBreaksAlone(t *testing.T) {
	t.Parallel()

	for name, document := range map[string]string{
		"a trailing backslash":    "first line\\\nsecond line\n",
		"two trailing spaces":     "first line  \nsecond line\n",
		"a backslash before crlf": "first line\\\r\nsecond line\r\n",
		"two spaces before crlf":  "first line  \r\nsecond line\r\n",
	} {
		assert.Empty(t, analyzeAuthored(t, document), "%s is a break this run was told to allow", name)
		assert.Len(t, analyze(t, "notes.md", document), 1, "%s is still reported by an unconfigured run", name)
	}
}

// TestAMarkupTagIsNotABreakThisRuleKnows pins the boundary of the lenient run:
// it recognises the FORMAT's spellings, not every construct a browser happens to
// render as a break. Raw markup in prose is a wrapped block in both runs.
func TestAMarkupTagIsNotABreakThisRuleKnows(t *testing.T) {
	t.Parallel()

	for _, tag := range []string{"<br>", "<br/>", "<br />", "<BR>"} {
		assert.Len(t, analyzeAuthored(t, "first line"+tag+"\nsecond line\n"), 1,
			"%s is not one of the format's own break spellings", tag)
	}
}

// TestOnlyTheUnauthoredBreaksOfAMixedBlockAreReported pins that the two kinds of
// break are judged separately within one paragraph when a run was told to allow
// one of them: the allowed break is left alone and the bare newline below it is
// still found.
func TestOnlyTheUnauthoredBreaksOfAMixedBlockAreReported(t *testing.T) {
	t.Parallel()

	diags := analyzeAuthored(t, "one\\\ntwo\nthree\n")

	require.Len(t, diags, 1)
	assert.Equal(t, []int{2}, linesOf(diags), "the backslash line is allowed, the bare newline below it is not")
}

// TestAConfiguredRunOnlyEverReportsLess pins the direction the setting moves in.
// A configuration that could make the rule report MORE than the default would be
// a second rule nobody reviewed; this one can only ever subtract, which is what
// makes the default the strictest run there is.
func TestAConfiguredRunOnlyEverReportsLess(t *testing.T) {
	t.Parallel()

	for _, document := range []string{
		"one\ntwo\n", "one\\\ntwo\n", "one  \ntwo\n", "one\\\ntwo\nthree\n", "- a\n  b\n",
		"> [!NOTE]\n> a\n> b\n", "| a | b |\n| - | - |\n| 1 | 2 |\n", "one<br>\ntwo\n",
	} {
		assert.LessOrEqual(t, len(analyzeAuthored(t, document)), len(analyze(t, "notes.md", document)),
			"the configured run reports no more than the default for %q", document)
	}
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

	// The question only arises in the run that was told to allow an authored
	// break: the default run reports every one of these, so it cannot tell a
	// visible break from an escaped backslash and has no need to.
	for run, wraps := range map[string]bool{
		`\`:      false,
		`\\`:     true,
		`\\\`:    false,
		`\\\\`:   true,
		`\\\\\`:  false,
		`\\\\\\`: true,
	} {
		diags := analyzeAuthored(t, "a line ending in "+run+"\nnext line\n")
		assert.Equal(t, wraps, len(diags) == 1, "%q trailing backslashes: wraps=%v", run, wraps)
		// The SAME question with a Windows line ending. The carriage return sits
		// between the backslashes and the newline, so a line read without
		// trimming it counts no backslashes at all and every one of these
		// becomes a finding — including the three that are visible breaks.
		crlf := analyzeAuthored(t, "a line ending in "+run+"\r\nnext line\r\n")
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

// TestACrlfBreakIsStillTheSameBreak pins that a Windows line ending — the shape
// most likely to lose it — changes neither run's answer: the carriage return
// sits between the spelling and the newline, and a line read without trimming it
// carries neither.
func TestACrlfBreakIsStillTheSameBreak(t *testing.T) {
	t.Parallel()

	for _, document := range []string{"first line\\\r\nsecond line\r\n", "first line  \r\nsecond line\r\n"} {
		assert.Empty(t, analyzeAuthored(t, document), "the run told to allow it allows it")
		assert.Len(t, analyze(t, "notes.md", document), 1, "and the unconfigured run reports it")
	}
}
