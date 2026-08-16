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

// TestConfiguredDocumentsCannotTurnTheDefaultSetOff drives the function its own
// doc comment makes claims about, and asserts the two of them that no other
// test states.
//
// The first is the additive one, in the direction that matters: the tests above
// prove the defaults survive a configuration naming something ELSE, which is
// the easy half. The half worth pinning is a configuration that names markdown
// itself — the only shape that could plausibly be read as "replace the set with
// this" — and the property is asserted as a SUPERSET over every default key
// rather than by naming .md twice, so a default added later is covered by this
// test on the day it is added. A configuration that could turn markdown off
// would be an opt-out from the whole rule, written in a repository's own file,
// which the rule never reads and no reviewer ever sees.
//
// Each case also asserts the file its OWN configuration newly reads, which is
// the other half of "plus whatever this run was told to read as well". Without
// it the table ran one set of assertions six times and observed nothing that
// made a case different: dropping the insertion entirely — parsing the list,
// refusing a path, adding nothing — left this test green while a repository
// that opted into .txt was silently read for markdown only. Found by an
// adversarial review of this test.
//
// The second claim is that an entry this run cannot apply is an ERROR and not
// an entry dropped: the sentinel is matched with errors.Is rather than by
// asking whether something went wrong, and the returned set is nil so no run
// can proceed on a configuration that was only partly applied.
func TestConfiguredDocumentsCannotTurnTheDefaultSetOff(t *testing.T) {
	t.Parallel()

	for name, adds := range map[string]struct{ configured, nowRead string }{
		"markdown by both its extensions": {".md, .markdown", "README.md"},
		"markdown cased the other way":    {".MD", "UPPER.MD"},
		"markdown beside something else":  {".md, .txt", "notes.txt"},
		"nothing at all":                  {"", "README.md"},
		"an unrelated extension":          {".rst", "guide.rst"},
		"a whole name":                    {"NOTES", "a/NOTES"},
	} {
		docs, err := hardwrap.ConfiguredDocuments(
			environment(map[string]string{documentsVariable: adds.configured}),
		)

		require.NoError(t, err, name)
		for key := range hardwrap.DefaultDocuments() {
			assert.True(t, docs[key], "%s leaves every default entry in place", name)
		}
		assert.True(t, docs.Reads("README.md"), "%s still reads markdown", name)
		assert.True(t, docs.Reads("docs/guide.markdown"), "%s still reads markdown's long spelling", name)
		assert.True(t, docs.Reads(hardwrap.Path(adds.nowRead)),
			"%s reads %s, so the entry was ADDED and not merely parsed", name, adds.nowRead)
	}

	docs, err := hardwrap.ConfiguredDocuments(
		environment(map[string]string{documentsVariable: ".txt, docs/notes.md"}),
	)

	require.ErrorIs(t, err, hardwrap.ErrDocumentPath,
		"an entry naming a path is refused with the shared sentinel, not with some other error")
	assert.Nil(t, docs,
		"and the whole configuration is refused: a partly-applied one is a run believing the opposite of what it does")
}

// TestOnlyALicenceIsExemptByItsName pins the licence table in the direction
// nothing in this repository pinned it: ADDING a stem.
//
// The removal direction was already covered — drop `licence` and the case above
// fails. The other direction was invisible: adding `"changelog": true` left
// `make check` at exit 0 and total coverage 100.0%, and a hard-wrapped
// CHANGELOG.md went silent. The exemption exists because a licence is a
// quotation of somebody else's words, and every other document named here is
// the repository's own to reflow. Found by an adversarial review.
func TestOnlyALicenceIsExemptByItsName(t *testing.T) {
	t.Parallel()

	for _, ours := range []string{"CHANGELOG.md", "README.md", "CONTRIBUTING.md", "SECURITY.md", "changelog.md"} {
		assert.Len(t, analyze(t, ours, "a paragraph that is\nwrapped\n"), 1,
			"%s is this repository's own prose", ours)
	}
}
