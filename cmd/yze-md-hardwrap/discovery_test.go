package main

// What this command claims, and nothing else. The walk itself — the symlinked
// root, the identity of a path reached two ways, the tree that cannot be read,
// the size bound, the ignore filter — belongs to the shared discovery and is
// proven there, once, rather than in every analyzer separately.

import (
	"os"
	"path/filepath"
	"syscall"
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
	docs, err := hardwrap.ConfiguredDocuments(func(name string) string {
		return map[string]string{"YZE_HARDWRAP_DOCUMENTS": ".txt"}[name]
	})
	require.NoError(t, err)
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

	withEnvironment(t, map[string]string{"YZE_HARDWRAP_DOCUMENTS": ".txt"})
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

// TestReportableKeepsOnlyADocumentOrATree pins the bound on the notice
// this command emits for a path the walk could not read.
//
// The shared walk reaches its unreadable arms BEFORE it asks whether an
// analyzer claims the path, so the list carries entries of any name and every
// one of them became an error-severity `yze/hardwrap` finding. Two of the 9,633
// findings a fleet sweep produced were a live ssh-agent socket and a dangling
// `.go` symlink — a rule about where a paragraph's newline goes has nothing to
// say about either, and neither has a remedy: an agent socket cannot be made
// readable as markdown, and the notice carries the same flat rule id as the wrap
// finding, so the only silence available switched off every genuine hard wrap in
// the repository too. A TREE is kept whatever it is called, because a directory
// nobody entered is where an unchecked document hides. Found by an adversarial
// review.
func TestReportableKeepsOnlyADocumentOrATree(t *testing.T) {
	dir := t.TempDir()
	tree := filepath.Join(dir, "locked")
	require.NoError(t, os.Mkdir(tree, 0o750))
	document := filepath.Join(dir, "notes.md")
	require.NoError(t, syscall.Mkfifo(document, 0o600))
	socket := filepath.Join(dir, "pipe.sock")
	require.NoError(t, syscall.Mkfifo(socket, 0o600))
	link := filepath.Join(dir, "dangling.go")
	require.NoError(t, os.Symlink(filepath.Join(dir, "nothing"), link))
	source := filepath.Join(dir, "main.go")
	require.NoError(t, os.WriteFile(source, []byte("package main\n"), 0o600))
	licence := filepath.Join(dir, "LICENSE.md")
	require.NoError(t, os.WriteFile(licence, []byte("a licence\n"), 0o600))

	kept := reportable([]string{document, socket, source, link, licence, tree}, hardwrap.DefaultDocuments())

	assert.Equal(t, []string{document, tree}, kept,
		"a document the gate could not open and a tree nobody entered; nothing else is this rule's business")
}

// TestAnUnreadableDocumentFollowsTheRunsConfiguration pins that the bound moves
// with the run's document set rather than with a list of its own, so a
// repository that opted `.txt` in hears about a `.txt` file the gate could not
// read.
func TestAnUnreadableDocumentFollowsTheRunsConfiguration(t *testing.T) {
	docs, err := hardwrap.ConfiguredDocuments(func(name string) string {
		return map[string]string{"YZE_HARDWRAP_DOCUMENTS": ".txt"}[name]
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"notes.txt"}, reportable([]string{"notes.txt", "main.go"}, docs))
}

// TestMayHoldDocumentsAsksTheFilesystemRatherThanTheName pins that the kind is asked of the FILESYSTEM
// rather than read off the name: a link to a directory is a tree the walk
// declined to enter, and a link resolving to nothing is not a tree at all.
func TestMayHoldDocumentsAsksTheFilesystemRatherThanTheName(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(target, 0o750))
	toTree := filepath.Join(dir, "elsewhere.go")
	require.NoError(t, os.Symlink(target, toTree))

	assert.Equal(t, []string{toTree}, reportable([]string{toTree}, hardwrap.DefaultDocuments()),
		"a link to a tree is a tree, whatever it is named")
}

// TestASubtreeNothingCanStatIsStillReported pins the arm whose absence dropped a
// whole subtree, and the documents inside it, in silence.
//
// A directory readable WITHOUT the execute bit lists its children and lets
// nothing stat them, so the walk hands back `a/b` and both lstat and stat fail
// with EACCES. Deciding "not a directory, so not this rule's business" from that
// failure meant a tree nobody entered was neither analyzed nor mentioned — the
// one outcome a gate must never produce, and it was introduced by the arm added
// to stop a socket being reported. Only a definite NO from the filesystem drops
// a path. Found by an adversarial review.
func TestASubtreeNothingCanStatIsStillReported(t *testing.T) {
	dir := t.TempDir()
	outer := filepath.Join(dir, "a")
	require.NoError(t, os.MkdirAll(filepath.Join(outer, "b"), 0o750))
	require.NoError(t, os.Chmod(outer, 0o400))
	t.Cleanup(func() { _ = os.Chmod(outer, 0o750) })

	kept := reportable([]string{filepath.Join(outer, "b")}, hardwrap.DefaultDocuments())

	assert.Equal(t, []string{filepath.Join(outer, "b")}, kept,
		"a path the filesystem will not answer for is not a path this rule may pass over")
}
