package main

// What this command claims, and nothing else. The walk itself — the symlinked
// root, the identity of a path reached two ways, the tree that cannot be read,
// the size bound, the ignore filter — belongs to the shared discovery and is
// proven there, once, rather than in every analyzer separately.

import (
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// TestDiscoveryClaimsWhatTheAnalyzerReads pins that the walk and the rule ask
// ONE question of one value. A walk that claimed a file the analyzer then
// declined to read would report nothing about it and say nothing about having
// skipped it.
func TestDiscoveryClaimsWhatTheAnalyzerReads(t *testing.T) {
	docs := hardwrap.ConfiguredDocuments(func(string) string { return ".txt" })
	claims := discovery(docs).Claims

	for _, path := range []string{"a/README.md", "a/guide.markdown", "a/UPPER.MD", "a/notes.txt"} {
		assert.True(t, claims(goyze.FilePath(path)), "%s is read", path)
		assert.True(t, docs.Reads(hardwrap.Path(path)), "%s is read by the analyzer too", path)
	}
	for _, path := range []string{"a/main.go", "a/Makefile", "a/image.png", "a/LICENSE.md"} {
		assert.False(t, claims(goyze.FilePath(path)), "%s is not", path)
		assert.False(t, docs.Reads(hardwrap.Path(path)), "%s is not read by the analyzer either", path)
	}
}

// TestOptingInReachesTheWalk pins the configuration end to end: a repository
// that asks for `.txt` gets its `.txt` files WALKED, not merely read when named.
func TestOptingInReachesTheWalk(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "notes.txt", wrapped)
	buf := swapStdout(t)

	require.Equal(t, 0, run([]string{dir}))
	assert.NotContains(t, buf.String(), "notes.txt", "unconfigured, plain text is not prose")

	original := lookupEnv
	lookupEnv = func(string) string { return ".txt" }
	t.Cleanup(func() { lookupEnv = original })
	buf = swapStdout(t)

	require.Equal(t, 0, run([]string{dir}))
	assert.Contains(t, buf.String(), "notes.txt", "configured, it is walked and judged")
}

// TestDiscoverySkipsSomebodyElsesProse pins the pruning. A dependency's or a
// theme's documentation is its own business, and reporting it tells this
// repository to reflow a file it does not own. `testdata` is here for a reason
// specific to this family: it is where an analyzer is proven in both
// directions, so a fixture that MUST contain a violation is not one.
func TestDiscoverySkipsSomebodyElsesProse(t *testing.T) {
	dir := t.TempDir()
	writeDoc(t, dir, "README.md", wrapped)
	for _, elsewhere := range []string{
		"node_modules/dep/README.md", "vendor/dep/README.md", "themes/paper/README.md",
		"testdata/fixture.md", ".venv/lib/pkg/README.md", "venv/lib/pkg/README.md",
		".tox/py/README.md", ".git/notes.md", "submodule/README.md",
	} {
		writeDoc(t, dir, elsewhere, wrapped)
	}
	writeDoc(t, dir, "submodule/.git/HEAD", "ref: refs/heads/main\n")
	buf := swapStdout(t)

	require.Equal(t, 0, run([]string{dir}))
	out := buf.String()
	assert.Contains(t, out, "README.md")
	for _, skipped := range append(somebodyElses(), ".git/", "submodule") {
		assert.NotContains(t, out, skipped, "%s is not this repository's prose", skipped)
	}
}

// somebodyElses is the expectation both pruning tests hold the walk to: the
// trees that belong to a dependency, a theme or a fixture in EVERY repository.
// It is written out here, beside the assertions, rather than read from the
// code — an expectation taken from the implementation asserts only that the
// implementation is itself.
func somebodyElses() []string {
	return []string{"vendor", "node_modules", "themes", "testdata", ".venv", "venv", ".tox"}
}

// TestPrunedNamesEveryTreeItClaimsTo pins the list itself rather than a sample:
// each name could be dropped with the suite green, and each drop makes somebody
// else's documentation this repository's problem.
func TestPrunedNamesEveryTreeItClaimsTo(t *testing.T) {
	for _, name := range somebodyElses() {
		assert.True(t, pruned(goyze.DirName(name)), "%s is somebody else's", name)
	}
	for _, name := range []string{"docs", "content", "internal", "cmd", "themes-guide"} {
		assert.False(t, pruned(goyze.DirName(name)), "%s is this repository's", name)
	}
}
