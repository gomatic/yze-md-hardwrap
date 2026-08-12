package hardwrap_test

// Which files a run reads. The default is the whole reason the rule is exact —
// a file is judged by what a markdown parser says its blocks are — so anything
// that is not markdown by name is opt-in, and the opt-in cannot turn the
// default off.

import (
	"testing"

	"github.com/stretchr/testify/assert"

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

	docs := hardwrap.ConfiguredDocuments(func(string) string { return ".txt" })

	assert.True(t, docs.Reads("notes.txt"), "the configured extension is read")
	assert.True(t, docs.Reads("README.md"), "and markdown is still read")
}

// TestAnEntryIsAnExtensionOrAWholeName pins the one vocabulary. An extensionless
// file cannot be opted in by extension — every Makefile, Dockerfile and binary
// shares that empty suffix — so it is named outright, and the two spellings are
// told apart by the dot they already carry.
func TestAnEntryIsAnExtensionOrAWholeName(t *testing.T) {
	t.Parallel()

	docs := hardwrap.ConfiguredDocuments(func(string) string { return ".txt, NOTES" })

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
		docs := hardwrap.ConfiguredDocuments(func(string) string { return configured })
		assert.True(t, docs.Reads("a.txt"), "%s names .txt", name)
		assert.True(t, docs.Reads("a.rst"), "%s names .rst", name)
	}
}

// TestAnUnsetConfigurationIsTheDefaultSet pins that an unconfigured run is the
// default rather than a nil surprise.
func TestAnUnsetConfigurationIsTheDefaultSet(t *testing.T) {
	t.Parallel()

	docs := hardwrap.ConfiguredDocuments(func(string) string { return "" })

	assert.Equal(t, hardwrap.DefaultDocuments(), docs)
}

// TestALicenceIsNeverRead pins the exemption that survives configuration. A
// licence is hard-wrapped by convention and is a quotation of somebody else's
// words — reflowing it changes a document whose whole value is being unchanged.
func TestALicenceIsNeverRead(t *testing.T) {
	t.Parallel()

	docs := hardwrap.ConfiguredDocuments(func(string) string { return "LICENSE, LICENCE, .txt" })

	for _, licence := range []string{"LICENSE", "licence", "LICENSE.md", "sub/LICENCE.txt", "License.markdown"} {
		assert.False(t, docs.Reads(hardwrap.Path(licence)), "%s is not ours to reflow", licence)
	}
	assert.True(t, docs.Reads("licensing.md"), "a document ABOUT licensing is ordinary prose")
}
