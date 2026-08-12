package hardwrap_test

// Which files a run reads. The default is the whole reason the rule is exact —
// a file is judged by what a markdown parser says its blocks are — so anything
// that is not markdown by name is opt-in, and the opt-in cannot turn the
// default off.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// TestMarkdownIsReadWithNoConfigurationAtAll pins the default set, in both
// directions: prose by its two extensions, and nothing else.
func TestMarkdownIsReadWithNoConfigurationAtAll(t *testing.T) {
	t.Parallel()

	docs := hardwrap.DefaultDocuments()

	for _, read := range []string{"README.md", "docs/guide.markdown", "UPPER.MD", "a/b/notes.Md"} {
		assert.True(t, docs.Reads(hardwrap.Path(read)), "%s is markdown", read)
	}
	for _, skipped := range []string{"notes.txt", "main.go", "Makefile", "CHANGELOG", "image.png", "notes.rst"} {
		assert.False(t, docs.Reads(hardwrap.Path(skipped)), "%s is opt-in, not default", skipped)
	}
}

// TestAConfiguredRunReadsMoreWithoutReadingLess pins that configuration ADDS.
// A setting that could turn markdown off would be an opt-out from the whole
// rule, written in a file the rule cannot see.
func TestAConfiguredRunReadsMoreWithoutReadingLess(t *testing.T) {
	t.Parallel()

	docs, err := hardwrap.ConfiguredDocuments(environment(map[string]string{documentsVariable: ".txt"}))

	require.NoError(t, err)
	assert.True(t, docs.Reads("notes.txt"), "the configured extension is read")
	assert.True(t, docs.Reads("README.md"), "and markdown is still read")
}

// TestAnEntryIsAnExtensionOrAWholeName pins the one vocabulary. An extensionless
// file cannot be opted in by extension — every Makefile, Dockerfile and binary
// shares that empty suffix — so it is named outright, and the two spellings are
// told apart by the dot they already carry.
func TestAnEntryIsAnExtensionOrAWholeName(t *testing.T) {
	t.Parallel()

	docs, err := hardwrap.ConfiguredDocuments(environment(map[string]string{documentsVariable: ".txt, NOTES"}))

	require.NoError(t, err)
	assert.True(t, docs.Reads("a/notes.txt"), "an extension matches any file carrying it")
	assert.True(t, docs.Reads("a/NOTES"), "a name matches that file")
	assert.True(t, docs.Reads("a/notes"), "however it is cased")
	assert.False(t, docs.Reads("a/NOTES.rst"), "a name is the WHOLE name, not a stem")
	assert.False(t, docs.Reads("a/Makefile"), "and nothing else extensionless comes with it")
}

// TestTheConfiguredListIsSplitTheWayItIsWritten pins that the delivered value
// is read in the shapes it arrives in: a comma-separated line, and the YAML
// block scalar that is the natural way to write a list in a runner's `env:`.
func TestTheConfiguredListIsSplitTheWayItIsWritten(t *testing.T) {
	t.Parallel()

	for name, configured := range map[string]string{
		"a comma-separated line": ".txt,.rst",
		"spaces":                 ".txt .rst",
		"a block scalar":         "\n.txt\n.rst\n",
		"both, untidily":         " .txt ,, \n .rst \n",
	} {
		docs, err := hardwrap.ConfiguredDocuments(environment(map[string]string{documentsVariable: configured}))
		require.NoError(t, err, name)
		assert.True(t, docs.Reads("a.txt"), "%s names .txt", name)
		assert.True(t, docs.Reads("a.rst"), "%s names .rst", name)
	}
}

// TestAnUnsetConfigurationIsTheDefaultSet pins that an unconfigured run is the
// default rather than a nil surprise.
func TestAnUnsetConfigurationIsTheDefaultSet(t *testing.T) {
	t.Parallel()

	docs, err := hardwrap.ConfiguredDocuments(environment(nil))

	require.NoError(t, err)
	assert.Equal(t, hardwrap.DefaultDocuments(), docs)
}

// TestALicenceIsNeverRead pins the exemption that survives configuration. A
// licence is hard-wrapped by convention and is a quotation of somebody else's
// words — reflowing it changes a document whose whole value is being unchanged.
func TestALicenceIsNeverRead(t *testing.T) {
	t.Parallel()

	docs, err := hardwrap.ConfiguredDocuments(
		environment(map[string]string{documentsVariable: "LICENSE, LICENCE, .txt"}),
	)
	require.NoError(t, err)

	for _, licence := range []string{"LICENSE", "licence", "LICENSE.md", "sub/LICENCE.txt", "License.markdown"} {
		assert.False(t, docs.Reads(hardwrap.Path(licence)), "%s is not ours to reflow", licence)
	}
	assert.True(t, docs.Reads("licensing.md"), "a document ABOUT licensing is ordinary prose")
}

// TestAnEntryNamingAPathIsRefused pins the one shape of entry that cannot work.
// A selector is matched against a file's whole name or its extension, so an
// entry carrying a separator claims nothing — and a configuration that claims
// nothing while reporting success is how a repository comes to believe it is
// being read.
func TestAnEntryNamingAPathIsRefused(t *testing.T) {
	t.Parallel()

	for _, configured := range []string{"docs/notes.txt", "./notes.txt", "a/b/c", ".txt, docs/notes.txt"} {
		docs, err := hardwrap.ConfiguredDocuments(environment(map[string]string{documentsVariable: configured}))

		require.ErrorIs(t, err, hardwrap.ErrDocumentPath, "%q names a path", configured)
		assert.Nil(t, docs, "a refused configuration yields no document set to run with")
	}
}

// TestASetNamingNothingReadsMarkdown pins the direction a forgotten field fails
// in, which an adversarial review found pointing the wrong way: the zero value
// of a [hardwrap.Settings] carries a nil document set, and a run that read no
// file under it would report a clean pass over every document in a repository —
// the one outcome a gate must never produce, reached by omitting a field rather
// than by any decision.
func TestASetNamingNothingReadsMarkdown(t *testing.T) {
	t.Parallel()

	for name, docs := range map[string]hardwrap.Documents{"nil": nil, "empty": {}} {
		assert.True(t, docs.Reads("README.md"), "a %s set is the default set, not a set that reads nothing", name)
		assert.False(t, docs.Reads("main.go"), "and it reads no more than the default one")
		assert.False(t, docs.Reads("LICENSE.md"), "a licence is still not ours to reflow")
	}
}
