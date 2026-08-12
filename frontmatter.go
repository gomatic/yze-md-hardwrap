package hardwrap

// Front matter: the one region of a markdown file that is not markdown.
//
// A Hugo content page opens with a fenced block of YAML or TOML metadata, and
// CommonMark has never heard of it. Parsed as markdown, `---` is a thematic
// break and the `key: value` lines beneath it are a paragraph — or, when a
// `---` closes the block, a SETEXT HEADING whose text spans every metadata
// line. That is a hard-wrapped block by this rule's own definition, and the
// fleet holds 1,948 such blocks, so leaving them in is not a rare false
// positive but a finding per content page.
//
// It is removed by TEXT rather than by an extension, and the removed lines are
// counted rather than blanked, so every finding below still names the line the
// author's editor shows.

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// lineOffset is how many lines were removed from the top of a document before
// it was parsed. Every reported line is measured in the ORIGINAL document, so
// this is added back to each one.
type lineOffset int

// The two front-matter delimiters. Hugo's third spelling, a bare JSON object,
// has no delimiter of its own — it opens with `{`, which is ordinary paragraph
// text in markdown, so recognising it would mean guessing where an object ends
// in a file that may simply begin with a brace. It is left alone deliberately:
// under-reading a rare shape is a silence, while mis-reading a common one is a
// finding on a line nobody wrote. (Fleet count of JSON front matter: zero.)
const (
	yamlFrontMatter = "---"
	tomlFrontMatter = "+++"
)

// withoutFrontMatter is a document's body and how many lines were taken off its
// top.
//
// Three things must hold, and the third is the one that matters. The opening
// delimiter must be the FIRST line; a closing one must exist; and the region
// between them must really BE the metadata language it claims to be.
//
// Without that last test, a delimiter pair is a two-line, audit-trail-free
// opt-out from the whole rule: wrapping any document in `---` … `---` costs two
// horizontal rules in the rendered output and deletes every finding in the file.
// It is also reachable by ACCIDENT, by a page that opens with a decorative
// thematic break and uses another one later. Found by an adversarial review;
// the guard that only required a closing delimiter did not cover either case.
func withoutFrontMatter(text Source) (Source, lineOffset) {
	first, rest, ok := nextLine(text)
	if !ok || !opensFrontMatter(first) {
		return text, 0
	}
	region, body, removed, closed := afterDelimiter(rest, delimiterOf(first))
	if !closed || !isMetadata(region, delimiterOf(first)) {
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

// isMetadata reports a region that really is front matter: the language its
// delimiter claims, not merely text that happens to sit between two of them.
//
// It is decided by PARSING first, because that is the exact answer: the region
// must be a MAPPING, which is the shape metadata takes, while prose wrapped in
// delimiters parses as a plain scalar or fails outright.
//
// Parsing alone is too strict, though, so a SHAPE test stands behind it.
// Twenty-one of the fleet's 1,947 YAML blocks do not decode — an unquoted colon
// inside a description, a `{{ }}` placeholder in a template archetype — and
// Hugo would refuse them too, but they are unmistakably somebody's front matter,
// and reading them as prose reports a hard-wrapped "heading" nobody wrote. What
// the shape still refuses is the thing that matters: wrapped PROSE between two
// delimiters, whose lines carry no key, no marker and no indent.
func isMetadata(region Source, of delimiter) bool {
	if of == tomlFrontMatter {
		return everyLineIsEntry(region, tomlEntry, tomlKey)
	}
	return parsesAsMapping(region) || everyLineIsEntry(region, yamlEntry, yamlKey)
}

// parsesAsMapping reports a region that decodes as a YAML mapping. An EMPTY
// region is one: `---` immediately closed is the empty metadata a generator
// writes, and it decodes to a nil map with no error.
func parsesAsMapping(region Source) bool {
	var mapping map[string]any
	return yaml.Unmarshal([]byte(region), &mapping) == nil
}

// The line shapes each metadata language is written in: a key, a comment, a
// table header, or the INDENTED continuation of any of them — a list entry
// among them, since a document's own keys sit at the margin and everything
// beneath one is indented. A bare `- ` at the margin is deliberately not a
// shape: it makes the region a YAML sequence rather than a mapping, which is not
// front matter in any generator, and accepting it let a wrapped list item hide
// between two delimiters.
//
// A TOML region has only this test, because a TOML parser is a dependency — and
// a file format the standards ban elsewhere — bought for the fleet's single
// `+++` block.
//
// The shapes are not enough on their own, which an adversarial review proved:
// INDENTATION is one of them, so prose indented by a single space between two
// `---` lines matched every line and deleted every finding in the region, while
// rendering as what it is — two horizontal rules and the same paragraphs.
// So a region must also CARRY A KEY at the margin. A document's own keys sit
// there and everything beneath one is indented; a region of nothing but
// continuation lines continues nothing.
var (
	yamlEntry = regexp.MustCompile(`^(?:[ \t]|#|[^\s:]+[ \t]*:)`)
	yamlKey   = regexp.MustCompile(`^[^\s:]+[ \t]*:`)
	tomlEntry = regexp.MustCompile(`^(?:[ \t]|#|\[[^\]]*\]|[^=\s\[]+[ \t]*=)`)
	tomlKey   = regexp.MustCompile(`^(?:\[[^\]]*\]|[^=\s\[]+[ \t]*=)`)
)

// everyLineIsEntry reports a region whose every non-blank line is one of its
// language's shapes AND which carries at least one key of its own.
func everyLineIsEntry(region Source, entry, key *regexp.Regexp) bool {
	isKeyed := false
	for _, text := range strings.Split(string(region), "\n") {
		if strings.TrimSpace(text) == "" {
			continue
		}
		if !entry.MatchString(text) {
			return false
		}
		isKeyed = isKeyed || key.MatchString(text)
	}
	return isKeyed
}

// afterDelimiter is the region up to the next line that is this delimiter alone,
// the text following it, how many lines were consumed reaching it, and whether
// it was found at all.
func afterDelimiter(text Source, closing delimiter) (Source, Source, lineOffset, bool) {
	region := strings.Builder{}
	for removed := lineOffset(0); ; {
		current, rest, ok := nextLine(text)
		if !ok {
			return "", "", 0, false
		}
		text = rest
		removed++
		if delimiterOf(current) == closing {
			return Source(region.String()), text, removed, true
		}
		// The builder never fails; its error exists for io.Writer's sake.
		_, _ = region.WriteString(string(current) + "\n")
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
