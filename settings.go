package hardwrap

// What one run of this rule was configured with, and the one place a run is
// allowed to be less strict than the standard.
//
// Both settings arrive the same way — an environment variable the stickler
// runner delivers from a repository's own `.stickler.yaml` — because one
// mechanism for the family is worth more than a marginally prettier second one,
// and because a repository's config is the only opt-in with an audit trail.

import (
	"strings"

	errs "github.com/gomatic/go-error"
)

// ErrBreaksSetting reports a break setting this rule does not define. An
// unrecognised value is REFUSED rather than read as the default: a run
// configured with a word this rule does not have would otherwise report the
// same as an unconfigured one, and nothing would say which of the two it was.
const ErrBreaksSetting errs.Const = "the break setting is not one this rule defines"

// Breaks says how this run reads a block that spans lines. The zero value is
// [StrictBreaks], so a caller that constructs a [Settings] without naming one
// gets the standard rather than the exception — the safe direction for a value
// nobody remembered to set.
type Breaks int

const (
	// StrictBreaks judges a block by the standard: one physical line, whatever
	// the source does to end its lines. It is the default because every
	// alternative spelling of a line ending is also a way to keep hard-wrapping
	// with the gate green, and a rule with a spelling that turns it off is a
	// rule that measures how well a writer knows the spelling.
	StrictBreaks Breaks = iota
	// AuthoredBreaks judges only the line endings the format discards, leaving
	// the ones it renders. It exists for a document whose breaks really are
	// authored — a poem, an address, a transcript — and it is a deliberate,
	// reviewable line in the repository that holds such a document rather than
	// a default, and a property of the run rather than of a line.
	AuthoredBreaks
)

// breaksVariable is the environment variable the break setting is read from. It
// is one variable for every spelling of an authored break rather than one per
// spelling: the spellings are one concept in the format itself, so a setting
// per spelling would be several names for one decision and several ways to
// enable half of it by accident.
const breaksVariable = "YZE_HARDWRAP_BREAKS"

// breakSetting is the value the environment carries, lower-cased and trimmed.
type breakSetting string

// The spellings the setting accepts. An unset variable and an explicit "strict"
// are the same run, so a repository can state the default it is relying on
// instead of relying on the absence of a line.
const (
	unsetSetting    breakSetting = ""
	strictSetting   breakSetting = "strict"
	authoredSetting breakSetting = "authored"
)

// Settings is everything one run was configured with: which files it reads, and
// how it reads a line ending. It is a value passed in rather than ambient
// state, so the analyzer, the command's discovery and every test agree by
// construction about what this run is.
type Settings struct {
	Documents Documents
	Breaks    Breaks
}

// DefaultSettings is the run with no configuration at all: markdown by its two
// extensions, judged by the standard.
func DefaultSettings() Settings {
	return Settings{Documents: DefaultDocuments(), Breaks: StrictBreaks}
}

// ConfiguredSettings is the run this environment describes, or the first refusal
// its configuration earns. A configuration that cannot be applied is an ERROR
// rather than a run that quietly does something else — a setting nobody can see
// take effect is indistinguishable from one nobody set.
func ConfiguredSettings(lookup func(string) string) (Settings, error) {
	docs, err := ConfiguredDocuments(lookup)
	if err != nil {
		return Settings{}, err
	}
	breaks, err := ConfiguredBreaks(lookup)
	if err != nil {
		return Settings{}, err
	}
	return Settings{Documents: docs, Breaks: breaks}, nil
}

// ConfiguredBreaks is how this run was told to read a line ending. It is a
// function of an injected lookup rather than a package variable so a test sees
// a configuration change without exporting one.
func ConfiguredBreaks(lookup func(string) string) (Breaks, error) {
	setting := breakSetting(strings.ToLower(strings.TrimSpace(lookup(breaksVariable))))
	switch setting {
	case unsetSetting, strictSetting:
		return StrictBreaks, nil
	case authoredSetting:
		return AuthoredBreaks, nil
	}
	return StrictBreaks, ErrBreaksSetting.With(nil, "variable", breaksVariable, "value", string(setting),
		"defined", string(strictSetting)+", "+string(authoredSetting))
}
