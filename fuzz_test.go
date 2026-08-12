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
