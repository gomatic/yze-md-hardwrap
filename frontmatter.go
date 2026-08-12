package hardwrap

// Front matter: the one region of a markdown file that is not markdown.
//
// A Hugo content page opens with a fenced block of YAML or TOML metadata, and
// CommonMark has never heard of it. Parsed as markdown, `---` is a thematic
// break and the `key: value` lines beneath it are a paragraph — or, when a
// `---` closes the block, a SETEXT HEADING whose text spans every metadata
// line. That is a hard-wrapped block by this rule's own definition, and it is
// present in every Hugo content file in the fleet, so leaving it in is not a
// rare false positive but a finding per content page.
//
// It is removed by TEXT rather than by an extension, and the removed lines are
// counted rather than blanked, so every finding below still names the line the
// author's editor shows.

import "strings"

// lineOffset is how many lines were removed from the top of a document before
// it was parsed. Every reported line is measured in the ORIGINAL document, so
// this is added back to each one.
type lineOffset int

// The two front-matter delimiters. Hugo's third spelling, a bare JSON object,
// has no delimiter of its own — it opens with `{`, which is ordinary paragraph
// text in markdown, so recognising it would mean guessing where an object ends
// in a file that may simply begin with a brace. It is left alone deliberately:
// under-reading a rare shape is a silence, while mis-reading a common one is a
// finding on a line nobody wrote.
const (
	yamlFrontMatter = "---"
	tomlFrontMatter = "+++"
)

// withoutFrontMatter is a document's body and how many lines were taken off its
// top.
//
// The opening delimiter must be the FIRST line and a closing one must exist,
// which is what keeps an ordinary thematic break from swallowing a document: a
// page whose body opens with `---` and never repeats it is returned untouched.
func withoutFrontMatter(text Source) (Source, lineOffset) {
	first, rest, ok := nextLine(text)
	if !ok || !opensFrontMatter(first) {
		return text, 0
	}
	body, removed, closed := afterDelimiter(rest, delimiterOf(first))
	if !closed {
		return text, 0
	}
	return body, removed + 1
}

// delimiter is the exact text a front-matter block opens and closes with.
type delimiter string

// opensFrontMatter reports a first line that is a front-matter delimiter and
// nothing else.
func opensFrontMatter(first line) bool {
	return delimiterOf(first) != ""
}

// delimiterOf is the front-matter delimiter this line is, or the empty
// delimiter when it is not one. The line must be the delimiter ALONE — trailing
// whitespace and a carriage return are tolerated because an editor leaves them,
// but `--- title` is prose.
func delimiterOf(text line) delimiter {
	trimmed := strings.TrimRight(string(text), " \t\r")
	if trimmed == yamlFrontMatter || trimmed == tomlFrontMatter {
		return delimiter(trimmed)
	}
	return ""
}

// afterDelimiter is the text following the next line that is exactly this
// delimiter, how many lines were consumed reaching it, and whether it was found
// at all.
func afterDelimiter(text Source, closing delimiter) (Source, lineOffset, bool) {
	for removed := lineOffset(0); ; {
		current, rest, ok := nextLine(text)
		if !ok {
			return "", 0, false
		}
		text = rest
		removed++
		if delimiterOf(current) == closing {
			return text, removed, true
		}
	}
}

// nextLine splits one line off the front of a document, reporting whether there
// was one. A final line with no newline after it is still a line; a document
// that has run out is not.
func nextLine(text Source) (line, Source, bool) {
	if text == "" {
		return "", "", false
	}
	at := strings.IndexByte(string(text), '\n')
	if at < 0 {
		return line(text), "", true
	}
	return line(text[:at]), text[at+1:], true
}
