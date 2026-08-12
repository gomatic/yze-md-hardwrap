package hardwrap_test

// Driving arbitrary text through the rule. The seeds are the edge matrix — the
// shapes that wrap, the shapes that merely look as though they do, the two
// spellings of an explicit break, front matter open and closed, and the line
// endings and invisible characters an editor introduces.

import (
	"strings"
	"testing"

	goyze "github.com/gomatic/go-yze"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// FuzzDiagnostics asserts, on every input rather than merely exercising:
//
//   - Diagnostics never panics, on any path or content, under either run.
//   - A tool failure carries no findings, so a refusal is never mistaken for a
//     clean pass over a document nobody read.
//   - Every diagnostic carries this rule's identity and a navigable position
//     that lies inside the document.
//   - The CONFIGURED run reports no more than the default one. That is the
//     property the whole design rests on: the setting can only ever subtract,
//     so no value of it and no shape of document can be stricter than an
//     unconfigured run, and the default is the strictest run there is.
//   - Under the configured run, a finding never names a line ending in one of
//     the format's own break spellings — the inverse of what that run promises,
//     held for every input rather than for the shapes a test thought of.
func FuzzDiagnostics(f *testing.F) {
	for _, seed := range []string{
		"", "\n", "\r\n", "one line\n", "one\ntwo\n", "one\\\ntwo\n", "one  \ntwo\n", "one\\\\\ntwo\n",
		"- a\n- b\n", "- a that\n  wraps\n", "> a\n> b\n", "| a | b |\n| - | - |\n| 1 | 2 |\n",
		"```\na\nb\n```\n", "~~~\na\nb\n~~~\n", "````\n```\na\nb\n```\n````\n", "    a\n    b\n",
		"<div>\na\n</div>\n", "<!--\na\nb\n-->\n", "<!-- @generated -->\n\na\nb\n",
		"[a]: http://a\n[b]: http://b\n", "text[^1]\n\n[^1]: a\n[^2]: b\n", "Term\n: value\n",
		"---\ntitle: x\n---\n\na\nb\n", "+++\ntitle = \"x\"\n+++\n\na\nb\n", "---\nnever closed\n\na\nb\n",
		"a title\nwrapped\n=======\n", "\ufeffa\nb\n", "a\r\nb\r\n", "a\\\r\nb\r\n", "a  \r\nb\r\n",
		"# One\n## Two\n", "a\n\n---\n\nb\n", "- [ ] a\n- [x] b\n", "text with `a code\nspan` inside\n",
		"> > a\n> > b\n", "> a\nlazy\n", "é\nè\n", "a\tb\nc\n",
	} {
		f.Add("notes.md", seed)
	}
	f.Add("LICENSE", "a\nb\n")
	f.Add("main.go", "a\nb\n")
	f.Add("guide.markdown", "a\nb\n")

	f.Fuzz(func(t *testing.T, path, source string) {
		strict := fuzzed(t, path, source, hardwrap.DefaultSettings())
		authored := fuzzed(t, path, source, hardwrap.Settings{
			Documents: hardwrap.DefaultDocuments(),
			Breaks:    hardwrap.AuthoredBreaks,
		})
		if len(authored) > len(strict) {
			t.Fatalf("the configured run reported %d findings where the default one reported %d",
				len(authored), len(strict))
		}
		lines := strings.Split(source, "\n")
		for _, diag := range strict {
			assertNavigable(t, diag, len(lines))
		}
		for _, diag := range authored {
			assertNavigable(t, diag, len(lines))
			assertNotAnExplicitBreak(t, diag, lines)
		}
	})
}

// fuzzed is one run's findings, holding the refusal contract: a tool failure
// carries no findings, so a refusal is never mistaken for a clean pass over a
// document nobody read.
func fuzzed(t *testing.T, path, source string, settings hardwrap.Settings) []goyze.Diagnostic {
	t.Helper()
	diags, err := hardwrap.Diagnostics(hardwrap.Path(path), hardwrap.Source(source), settings)
	if err != nil && len(diags) != 0 {
		t.Fatalf("a tool failure yields no findings, got %d", len(diags))
	}
	if err != nil {
		return nil
	}
	return diags
}

// assertNavigable pins that a finding can be opened: this rule's identity, and
// a 1-based position inside the document it names.
func assertNavigable(t *testing.T, diag goyze.Diagnostic, count int) {
	t.Helper()
	if diag.Rule != hardwrap.Rule || diag.Tool != hardwrap.Tool {
		t.Fatalf("diagnostic carries %q/%q rather than this rule's identity", diag.Tool, diag.Rule)
	}
	if diag.Line < 1 || diag.Col != 1 {
		t.Fatalf("position %d:%d is not navigable", diag.Line, diag.Col)
	}
	if diag.Line > count {
		t.Fatalf("line %d is past the document's %d lines", diag.Line, count)
	}
}

// assertNotAnExplicitBreak pins what the CONFIGURED run promises. A line ending
// in two spaces, or in an odd run of backslashes, is one of the format's own
// break spellings; a run told to allow those and then reporting one would leave
// its own configuration meaning nothing.
func assertNotAnExplicitBreak(t *testing.T, diag goyze.Diagnostic, lines []string) {
	t.Helper()
	if !strings.Contains(diag.Message, wrapMarker) {
		return
	}
	text := strings.TrimSuffix(lines[diag.Line-1], "\r")
	if strings.HasSuffix(text, "  ") {
		t.Fatalf("line %d ends in two spaces, which is an explicit break: %q", diag.Line, text)
	}
	if backslashes(text)%2 == 1 {
		t.Fatalf("line %d ends in a backslash, which is an explicit break: %q", diag.Line, text)
	}
}

// backslashes is the length of the run of backslashes a line ends with. An even
// run is escaped backslashes and ordinary text; an odd one ends in a break.
func backslashes(text string) int {
	return len(text) - len(strings.TrimRight(text, `\`))
}

// FuzzWrappedProseIsAlwaysFound drives the PRESENCE direction, which the target
// above cannot reach: it asserts only that nothing wrong is reported, and a rule
// that reported nothing at all would satisfy it completely. That is not a
// theoretical gap — the two worst defects this analyzer has had were both
// SILENCES, a delimiter pair that swallowed a whole document and a claim that
// exempted one.
//
// The property: a hard-wrapped paragraph appended to an arbitrary HEADER is
// reported, for every header that cannot legitimately absorb it. A header is
// disqualified only by shapes that genuinely extend to the end of a document —
// an unterminated fence or HTML comment, and an indent that would make the
// canary code — because those swallow the canary by the rule's own contract
// rather than by a defect.
func FuzzWrappedProseIsAlwaysFound(f *testing.F) {
	for _, header := range []string{
		"", "# Title\n", "---\ntitle: x\n---\n", "+++\ntitle = \"x\"\n+++\n", "---\n\nprose\n\n---\n",
		"<!-- @generated -->\n", "<!-- --><span title=\"@generated\"></span>\n", "<!-- x --> @generated\n",
		"```\nfenced\n```\n", "| a | b |\n| - | - |\n", "> quote\n", "- item\n", "[a]: http://a\n",
		"<div>\nhtml\n</div>\n", "text\n", "\ufeff# Title\n", "---\r\ntitle: x\r\n---\r\n",
	} {
		f.Add(header)
	}

	f.Fuzz(func(t *testing.T, header string) {
		if swallows(header) {
			return
		}
		for _, canary := range canaries {
			assertCanaryIsFound(t, header, canary)
		}
	})
}

// canaries are the wrapped paragraph in every spelling of a line ending there
// is: a bare newline, and each of the spellings the format renders. The default
// run must find ALL of them — a spelling that goes unreported is not a nuance,
// it is the rule's off switch, available per line and needing no configuration.
var canaries = map[string]string{
	"a bare newline":        "the canary paragraph that is\nwrapped over two lines\n",
	"two trailing spaces":   "the canary paragraph that is  \nwrapped over two lines\n",
	"a trailing backslash":  "the canary paragraph that is\\\nwrapped over two lines\n",
	"a run of backslashes":  "the canary paragraph that is\\\\\\\nwrapped over two lines\n",
	"a trailing markup tag": "the canary paragraph that is<br>\nwrapped over two lines\n",
}

// assertCanaryIsFound pins that a wrapped paragraph beneath a header is
// reported by the default run.
func assertCanaryIsFound(t *testing.T, header, canary string) {
	t.Helper()
	diags, err := hardwrap.Diagnostics("notes.md", hardwrap.Source(header+"\n\n"+canary),
		hardwrap.DefaultSettings())
	if err != nil {
		return
	}
	for _, diag := range diags {
		if strings.Contains(diag.Message, wrapMarker) {
			return
		}
	}
	t.Fatalf("a hard-wrapped paragraph spelled %q went unreported after header %q", canary, header)
}

// swallows reports a header that legitimately absorbs everything after it, so
// the canary below it is not prose this rule judges.
//
// It is a deliberately WIDE net rather than a copy of the analyzer's own tests:
// a precondition that mirrors the implementation shares its mistakes, and this
// one only ever skips cases — over-excluding costs the property some power,
// while under-excluding would make it assert something untrue.
func swallows(header string) bool {
	lowered := strings.ToLower(header)
	for _, opener := range absorbing {
		if strings.Contains(lowered, opener) {
			return true
		}
	}
	// A generated document is out of scope ENTIRELY, by the ratified rule.
	if strings.Contains(lowered, "@generated") || strings.Contains(lowered, "do not edit") {
		return true
	}
	// An indent makes what follows a code block rather than a paragraph.
	return strings.HasPrefix(header, "    ") || strings.HasPrefix(header, "\t") ||
		strings.Contains(header, "\n    ") || strings.Contains(header, "\n\t")
}

// absorbing are the constructs whose content continues past a blank line, so
// that a canary beneath one is inside it rather than beneath it.
//
// Two families. The fences, whose closing run must be at least as long as the
// opening one — which is why counting delimiters is not enough: ```` ``````0 ````
// opens a six-backtick fence that three backticks would not close. And HTML
// blocks of CommonMark's types 1 to 5, which end at a specific string rather
// than at a blank line and so run to the end of the document when that string
// never comes: `<script>`, `<?`, and every `<!` declaration — `<!A00` is one,
// and it swallowed a canary that a `<!--` test alone did not cover.
var absorbing = []string{"```", "~~~", "<!", "<?", "<pre", "<script", "<style", "<textarea"}
