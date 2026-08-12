package hardwrap

// Which of a block's line breaks a run REPORTS.
//
// The parse says where a block's lines end; this file says which of those
// endings a finding is made of. There are two answers and they are not the same
// kind of answer: a GitHub alert marker is structural in every run, because
// joining it deletes a construct rather than reflowing prose; the format's own
// spellings for a break meant to be seen are left alone only where the run was
// configured to leave them, and never by default. See [Breaks] for why the
// default is the strict one.

import (
	"strings"

	"github.com/yuin/goldmark/ast"
)

// reported is the breaks this run reports, which is every break in the block
// unless it was configured otherwise or the break is structural.
func (d scanned) reported(node ast.Node, breaks []lineBreak) []byteOffset {
	kept := make([]byteOffset, 0, len(breaks))
	for _, at := range breaks {
		if d.reports(node, at) {
			kept = append(kept, at.at)
		}
	}
	return kept
}

// reports is the decision itself, for one break.
//
// A GitHub alert marker is left alone in EVERY run: it is not a way of writing
// a line ending, it is a construct whose first newline is structural, and
// joining it deletes the alert. Everything else depends on what the run was
// configured with — see [Breaks], and note that the default configuration
// leaves nothing alone, which is the whole point of it.
func (d scanned) reports(node ast.Node, at lineBreak) bool {
	text := d.lineAt(at.at)
	if d.opensAlert(node, at.at, text) {
		return false
	}
	return d.breaks != AuthoredBreaks || !at.isAuthored(text)
}

// isAuthored reports a break the FORMAT spells as one meant to be seen.
//
// The parser has already answered most of it. What it gets wrong is a run of
// backslashes: CommonMark resolves the escapes first, so an odd run leaves a
// lone backslash against the newline and the break is visible, while goldmark
// decides from the last two characters and calls a run of three escaped text.
func (b lineBreak) isAuthored(text line) bool {
	return bool(b.isHard) || trailingBackslashes(text)%2 == 1
}

// alertMarkers are the five GitHub alert types. The marker must stand ALONE on
// the first line of its blockquote, so the newline after it is structural: join
// it and the alert becomes an ordinary quotation, losing its icon, its colour
// and its meaning. It is invisible in plain CommonMark and visible everywhere
// this fleet's markdown is actually rendered.
//
// Measured before it was mechanized (2026-08-12): nine markers in seven files
// fleet-wide, every one of them third-party — a Hugo theme, vendored provider
// changelogs, spec-kit command templates — and two of them were reported. A
// false positive that instructs an author to break a correct document is worth
// ten lines whatever its count, and this construct is common everywhere else.
var alertMarkers = map[line]bool{
	"[!NOTE]": true, "[!TIP]": true, "[!IMPORTANT]": true, "[!WARNING]": true, "[!CAUTION]": true,
}

// opensAlert reports the marker line of a GitHub alert.
//
// Three conditions, and the third was a hole an adversarial review found: the
// block sits in a blockquote (`[!NOTE]` in ordinary prose is a bracketed word,
// and its newline really is decorative), the marker is the type GitHub defines,
// and it is on the block's FIRST line. Only the first line of a blockquote
// opens an alert; a second `> [!NOTE]` below it is literal body text, and
// exempting that one silenced a genuine wrap.
func (d scanned) opensAlert(node ast.Node, at byteOffset, text line) bool {
	parent := node.Parent()
	// The blockquote's own markers are stripped because the line is read from
	// the RAW document — the parser's segments have them removed already, but
	// the raw line is what a position maps back to, and `> [!NOTE]` is the
	// spelling on disk.
	quoted := strings.TrimLeft(string(text), " \t>")
	return parent != nil && parent.Kind() == ast.KindBlockquote && d.opensBlock(node, at) &&
		alertMarkers[line(strings.ToUpper(strings.TrimSpace(quoted)))]
}

// opensBlock reports a position lying on the first line of its block.
func (d scanned) opensBlock(node ast.Node, at byteOffset) bool {
	lines := node.Lines()
	return lines.Len() > 0 && d.lines.of(byteOffset(lines.At(0).Start)) == d.lines.of(at)
}

// lineAt is the text of the line holding an offset, without its ending.
func (d scanned) lineAt(at byteOffset) line {
	found := d.lines.of(at)
	start := d.lines[found-1]
	end := byteOffset(len(d.source))
	if int(found) < len(d.lines) {
		// One before the next line's start is the newline itself, which is not
		// part of the line the author wrote.
		end = d.lines[found] - 1
	}
	return line(strings.TrimSuffix(string(d.source[start:end]), "\r"))
}

// trailingBackslashes is the length of the run of backslashes a line ends with.
// An even run is escaped backslashes and ordinary text; an odd one leaves a
// lone backslash against the newline, which is a break the author asked to be
// seen.
func trailingBackslashes(text line) int {
	return len(text) - len(strings.TrimRight(string(text), `\`))
}
