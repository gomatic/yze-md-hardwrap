package hardwrap

// Which files this rule reads, and how a run is told to read more of them.
//
// Markdown is the default and everything else is opt-in, because the rule's
// exactness comes from parsing: a file is judged by what a CommonMark parser
// says its blocks are, and that answer is only meaningful for a file that IS
// markdown. There is no plain-text mode and no heuristic fallback — an enabled
// file is parsed as markdown, whatever it is called.

import (
	"path"
	"strings"
	"unicode"

	errs "github.com/gomatic/go-error"
)

// ErrDocumentPath reports a configured entry that names a path. An entry is
// matched against a file's whole NAME or its extension, neither of which can
// hold a separator, so `docs/notes.txt` claims nothing — and claiming nothing
// silently is how a repository comes to believe it is being read. It is refused
// rather than trimmed to the name it contains, because the two mean different
// things and only the author of the line knows which was meant.
const ErrDocumentPath errs.Const = "a document entry is a file name or an extension, never a path"

// selector is one lower-cased spelling a document is claimed by: an extension
// (".md") or a whole file name ("NOTES").
//
// Both live in ONE set and are matched against both spellings of a path,
// because the distinction is already in the text: a name is confusable with an
// extension only when it begins with a dot, and a file that begins with a dot
// has the same string for both. Splitting them into two configured lists would
// be two vocabularies for one question.
type selector string

// baseName is a file's final path element, lower-cased.
type baseName string

// Documents is the set of file kinds a run reads. It is a value passed in
// rather than ambient state, so the analyzer, the command's discovery and every
// test agree about which files are in scope by construction — a walk that
// claims a file the analyzer then declines to read reports nothing about it and
// says nothing about having skipped it.
type Documents map[selector]bool

// The extensions that are markdown by name, and so the whole default. An
// analyzer reading `.txt` or every extensionless file out of the box would parse
// Makefiles, licences and binaries as prose.
const (
	markdownExt     selector = ".md"
	markdownLongExt selector = ".markdown"
)

// documentsVariable is the environment variable a run is configured from: a
// comma- or whitespace-separated list of extensions (".txt") and whole file
// names ("NOTES"), each of which is read AS MARKDOWN in addition to the
// defaults.
//
// The environment is the configuration surface because it is the one the
// stickler runner already has for a tool it invokes as a subprocess: a repo's
// .stickler.yaml sets it through the `env:` of the runner spec it defines, so
// opting a repository into `.txt` is a line of that repository's own config and
// no change here. It is also how the sibling analyzer yze-yml-wfpolicy is
// configured, and one mechanism for the family is worth more than a marginally
// prettier second one.
const documentsVariable = "YZE_HARDWRAP_DOCUMENTS"

// licenceStems are the documents that are hard-wrapped by convention and are
// not the repository's to reflow. A licence's text is a quotation of somebody
// else's words — reflowing it changes a document whose whole value is being
// unchanged — so it is exempt whatever it is called next and however a run is
// configured. Both spellings are named because both are in use, and a rule that
// exempted one would report the other for the same text.
var licenceStems = map[baseName]bool{"license": true, "licence": true}

// DefaultDocuments is the set read with no configuration at all: markdown, by
// its two extensions.
func DefaultDocuments() Documents {
	return Documents{markdownExt: true, markdownLongExt: true}
}

// ConfiguredDocuments is [DefaultDocuments] plus whatever this run was told to
// read as well. It is a function of an injected lookup rather than a package
// variable so a test sees a configuration change without exporting one, and so
// an unset variable is plainly the default set rather than a nil surprise.
//
// The defaults are ADDED to rather than replaced. A configuration that could
// turn markdown off would let a repository opt out of the rule entirely from a
// file the rule itself cannot see, which is the audit-trail-free opt-out every
// gate has to refuse.
//
// An entry this run cannot apply is an ERROR rather than an entry ignored. See
// [ErrDocumentPath]: configuration that does nothing and says nothing is the
// worst of the three possible outcomes, because the repository that wrote it
// believes the opposite of what is happening.
func ConfiguredDocuments(lookup func(string) string) (Documents, error) {
	docs := DefaultDocuments()
	configured := lookup(documentsVariable)
	// No emptiness guard: a field from FieldsFunc is by construction a run of
	// non-separator characters, and whitespace IS a separator here, so a field
	// can be neither empty nor surrounded by space. A guard against it would be
	// a condition no input can make false — a check that looks like one and
	// tests nothing.
	for _, entry := range strings.FieldsFunc(configured, func(r rune) bool { return isEntrySeparator(separatorRune(r)) }) {
		if strings.ContainsRune(entry, pathSeparator) {
			return nil, ErrDocumentPath.With(nil, "variable", documentsVariable, "entry", entry)
		}
		docs[selector(strings.ToLower(entry))] = true
	}
	return docs, nil
}

// pathSeparator is the character that makes an entry a path rather than a name.
const pathSeparator = '/'

// separatorRune is one character standing between two entries of the list.
type separatorRune rune

// isEntrySeparator reports the characters that stand between two entries of the
// configured list. Whitespace separates as well as a comma, because the
// documented way to deliver this value is a YAML `env:` entry, where a block
// scalar — one entry per line — is the natural shape for a list.
func isEntrySeparator(between separatorRune) bool {
	return between == ',' || unicode.IsSpace(rune(between))
}

// Reads reports a path this run analyzes: a licence never, and otherwise any
// file whose whole name or whose extension was configured.
func (d Documents) Reads(at Path) bool {
	base := baseName(strings.ToLower(path.Base(string(at))))
	if licenceStems[stemOf(base)] {
		return false
	}
	return d[selector(base)] || d[selector(path.Ext(string(base)))]
}

// stemOf is a file name without its extension, which is what a licence is
// recognised by: `LICENSE`, `LICENSE.md` and `LICENSE.txt` are one document in
// three spellings.
func stemOf(base baseName) baseName {
	return baseName(strings.TrimSuffix(string(base), path.Ext(string(base))))
}
