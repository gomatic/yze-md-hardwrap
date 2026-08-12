package hardwrap_test

// What a run was configured with. Two properties matter more than the parsing:
// an absent or unreadable configuration is the STRICT run, never the lenient
// one; and a configured value this rule cannot apply is refused out loud rather
// than dropped on the way past.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// The environment variables this analyzer reads, spelled here as a consumer
// spells them in a `.stickler.yaml`. They are duplicated deliberately: the
// package's own constants are unexported, so a test that imported them could
// not tell a rename from a working run, and these names are a published
// contract that a repository's configuration is written against.
const (
	documentsVariable = "YZE_HARDWRAP_DOCUMENTS"
	breaksVariable    = "YZE_HARDWRAP_BREAKS"
)

// environment is a lookup over a fixed set of variables, so a test configures
// one without exporting anything into the process — and so a lookup that
// ignores which variable it was asked about cannot make a test pass for the
// wrong reason.
func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

// TestAnUnconfiguredRunIsStrict pins the default, which is the whole point of
// the setting: a run nobody configured judges every line ending.
func TestAnUnconfiguredRunIsStrict(t *testing.T) {
	t.Parallel()

	settings, err := hardwrap.ConfiguredSettings(environment(nil))

	require.NoError(t, err)
	assert.Equal(t, hardwrap.StrictBreaks, settings.Breaks)
	assert.Equal(t, hardwrap.DefaultSettings(), settings)
}

// TestTheZeroSettingsValueIsTheStrictRun pins the direction a forgotten field
// fails in. A caller that builds a [hardwrap.Settings] and names no break
// setting gets the standard, never the exception.
func TestTheZeroSettingsValueIsTheStrictRun(t *testing.T) {
	t.Parallel()

	var settings hardwrap.Settings

	assert.Equal(t, hardwrap.StrictBreaks, settings.Breaks)
}

// TestTheBreakSettingIsReadInTheSpellingsItIsWritten pins the two values, and
// pins that an explicit "strict" is the same run as an unset variable — so a
// repository can state the default it relies on instead of relying on the
// absence of a line.
func TestTheBreakSettingIsReadInTheSpellingsItIsWritten(t *testing.T) {
	t.Parallel()

	for configured, want := range map[string]hardwrap.Breaks{
		"":           hardwrap.StrictBreaks,
		"strict":     hardwrap.StrictBreaks,
		"  strict  ": hardwrap.StrictBreaks,
		"authored":   hardwrap.AuthoredBreaks,
		"AUTHORED":   hardwrap.AuthoredBreaks,
		"\nauthored": hardwrap.AuthoredBreaks,
	} {
		breaks, err := hardwrap.ConfiguredBreaks(environment(map[string]string{breaksVariable: configured}))
		require.NoError(t, err, "%q is a value this rule defines", configured)
		assert.Equal(t, want, breaks, "%q", configured)
	}
}

// TestAnUndefinedBreakSettingIsRefusedRatherThanRead pins the refusal, and pins
// WHICH run a refused configuration would otherwise have been: every one of
// these spellings is somebody trying to turn the exception on, and reading them
// as the default would leave the repository believing the opposite of what runs.
func TestAnUndefinedBreakSettingIsRefusedRatherThanRead(t *testing.T) {
	t.Parallel()

	for _, configured := range []string{"true", "1", "on", "yes", "authored breaks", "lenient", "STRICTER"} {
		breaks, err := hardwrap.ConfiguredBreaks(environment(map[string]string{breaksVariable: configured}))

		require.ErrorIs(t, err, hardwrap.ErrBreaksSetting, "%q is not a value this rule defines", configured)
		assert.Equal(t, hardwrap.StrictBreaks, breaks, "and a refusal never yields the lenient run")
	}
}

// TestConfiguredSettingsRefusesWhateverItsPartsRefuse pins that neither
// variable's refusal is swallowed by the other's success, and that a refused
// configuration yields no usable settings at all.
func TestConfiguredSettingsRefusesWhateverItsPartsRefuse(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		values  map[string]string
		wantErr error
	}{
		"a document entry naming a path": {
			values:  map[string]string{documentsVariable: "docs/notes.txt"},
			wantErr: hardwrap.ErrDocumentPath,
		},
		"a break setting this rule never had": {
			values:  map[string]string{breaksVariable: "off"},
			wantErr: hardwrap.ErrBreaksSetting,
		},
		"both, refused at the first": {
			values:  map[string]string{documentsVariable: "a/b", breaksVariable: "off"},
			wantErr: hardwrap.ErrDocumentPath,
		},
		"neither": {
			values:  map[string]string{documentsVariable: ".txt", breaksVariable: "authored"},
			wantErr: nil,
		},
	} {
		settings, err := hardwrap.ConfiguredSettings(environment(testCase.values))

		require.ErrorIs(t, err, testCase.wantErr, name)
		if testCase.wantErr != nil {
			assert.Equal(t, hardwrap.Settings{}, settings, "%s yields nothing a run could be made from", name)
		}
	}
}

// TestAConfiguredRunReadsTheVariablesItDocuments pins the exact spellings, which
// are a published contract: a repository's `.stickler.yaml` names them, and a
// renamed variable would go inert with every configured run silently reverting
// to the default.
func TestAConfiguredRunReadsTheVariablesItDocuments(t *testing.T) {
	t.Parallel()

	asked := map[string]bool{}
	settings, err := hardwrap.ConfiguredSettings(func(name string) string {
		asked[name] = true
		return map[string]string{documentsVariable: ".txt", breaksVariable: "authored"}[name]
	})

	require.NoError(t, err)
	assert.Equal(t, map[string]bool{documentsVariable: true, breaksVariable: true}, asked,
		"two variables, spelled exactly this way")
	assert.True(t, settings.Documents.Reads("notes.txt"))
	assert.Equal(t, hardwrap.AuthoredBreaks, settings.Breaks)
}
