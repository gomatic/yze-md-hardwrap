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
//   - Diagnostics never panics, on any path or content.
//   - A tool failure carries no findings, so a refusal is never mistaken for a
//     clean pass over a document nobody read.
//   - Every diagnostic carries this rule's identity and a navigable position
//     that lies inside the document.
//   - A hard-wrapped finding never names a line the author ended with an
//     EXPLICIT break. That is the inverse of the whole rule: the escape hatch
//     the format provides has to hold for every input, not for the shapes a
//     test happened to think of.
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
		diags, err := hardwrap.Diagnostics(hardwrap.Path(path), hardwrap.Source(source), hardwrap.DefaultDocuments())
		if err != nil {
			if len(diags) != 0 {
				t.Fatalf("a tool failure yields no findings, got %d", len(diags))
			}
			return
		}
		lines := strings.Split(source, "\n")
		for _, diag := range diags {
			assertNavigable(t, diag, len(lines))
			assertNotAnExplicitBreak(t, diag, lines)
		}
	})
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

// assertNotAnExplicitBreak pins the inverse of the rule. A line ending in two
// spaces, or in an odd run of backslashes, carries a break the author asked to
// be SEEN — reporting one would leave no way to write a visible line break at
// all.
func assertNotAnExplicitBreak(t *testing.T, diag goyze.Diagnostic, lines []string) {
	t.Helper()
	if !strings.Contains(diag.Message, "is hard-wrapped") {
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
		const canary = "the canary paragraph that is\nwrapped over two lines\n"
		diags, err := hardwrap.Diagnostics("notes.md", hardwrap.Source(header+"\n\n"+canary),
			hardwrap.DefaultDocuments())
		if err != nil {
			return
		}
		for _, diag := range diags {
			if strings.Contains(diag.Message, "is hard-wrapped") {
				return
			}
		}
		t.Fatalf("a hard-wrapped paragraph went unreported after header %q", header)
	})
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
