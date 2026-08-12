package main

// The ambient input this command has, the seam it is read through, and what
// happens to a line of it this command cannot apply.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// withEnvironment points the command's one seam at a fixed set of variables for
// the duration of a test, restoring the neutral lookup afterwards so tests
// cannot leak into one another.
func withEnvironment(t *testing.T, values map[string]string) {
	t.Helper()
	original := lookupEnv
	lookupEnv = func(name string) string { return values[name] }
	t.Cleanup(func() { lookupEnv = original })
}

// TestConfigurationIsReadThroughTheSeam pins that the environment reaches both
// settings — and that it does so through an injected lookup, so a test sees a
// configuration change without exporting one into the process.
func TestConfigurationIsReadThroughTheSeam(t *testing.T) {
	withEnvironment(t, map[string]string{
		"YZE_HARDWRAP_DOCUMENTS": ".txt",
		"YZE_HARDWRAP_BREAKS":    "authored",
	})

	settings, err := configuredSettings()

	require.NoError(t, err)
	assert.True(t, settings.Documents.Reads("notes.txt"), "the configured kind is read")
	assert.True(t, settings.Documents.Reads("README.md"), "and markdown still is")
	assert.Equal(t, hardwrap.AuthoredBreaks, settings.Breaks, "and the configured run is the one asked for")
}

// TestAnUnconfiguredRunIsTheDefaultRun pins the default at the command's edge:
// an unset environment is the default document set judged by the standard, not
// an empty set that reads nothing nor a lenient run nobody asked for.
func TestAnUnconfiguredRunIsTheDefaultRun(t *testing.T) {
	settings, err := configuredSettings()

	require.NoError(t, err)
	assert.Equal(t, hardwrap.DefaultSettings(), settings)
}

// TestConfiguredSettingsFailsTheRunOnAConfigurationItCannotApply pins the
// refusal reaching the exit code. A run that dropped the line and reported
// anyway would produce the output of an unconfigured run, and the repository
// would have no way to learn which of the two it got.
func TestConfiguredSettingsFailsTheRunOnAConfigurationItCannotApply(t *testing.T) {
	for name, testCase := range map[string]struct {
		values  map[string]string
		wantErr error
	}{
		"a document entry naming a path": {
			values:  map[string]string{"YZE_HARDWRAP_DOCUMENTS": "docs/notes.txt"},
			wantErr: hardwrap.ErrDocumentPath,
		},
		"a break setting this rule never had": {
			values:  map[string]string{"YZE_HARDWRAP_BREAKS": "true"},
			wantErr: hardwrap.ErrBreaksSetting,
		},
	} {
		withEnvironment(t, testCase.values)
		dir := t.TempDir()
		writeDoc(t, dir, "README.md", wrapped)

		require.ErrorIs(t, report([]string{dir}), testCase.wantErr, name)
		assert.Equal(t, 1, run([]string{dir}), "%s exits non-zero", name)
	}
}
