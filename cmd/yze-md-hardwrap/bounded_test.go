package main

// What this command refuses to read. The bound sits at the single place every
// path is read, so the walk and a named argument cannot disagree about it — and
// it bounds the READ rather than a size asked for beforehand, which describes
// the path rather than the bytes behind it.

import (
	"os"
	"path/filepath"
	"testing"

	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// TestReadFileBoundsEveryPathItReads pins both sides of the boundary, so the
// limit cannot be moved in either direction without a failure.
func TestReadFileBoundsEveryPathItReads(t *testing.T) {
	dir := t.TempDir()

	for name, size := range map[string]goyze.ByteCount{
		"at the limit": hardwrap.SizeLimit,
		"over it":      hardwrap.SizeLimit + 1,
	} {
		at := filepath.Join(dir, name+".md")
		require.NoError(t, os.WriteFile(at, nil, 0o600))
		require.NoError(t, os.Truncate(at, int64(size)))

		data, err := readFile(at)

		if size > hardwrap.SizeLimit {
			assert.ErrorIs(t, err, goyze.ErrTooLarge, name)
			continue
		}
		require.NoError(t, err, name)
		assert.Len(t, data, int(size), name)
	}
}

// TestReadFileRefusesAnOversizeDocumentThroughASymlink pins the bound against
// the path that defeats a stat: a link's own few bytes describe the link, not
// the document behind it, and the walk deliberately follows symlinks — so this
// is an ordinary document, not a contrived one.
func TestReadFileRefusesAnOversizeDocumentThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	huge := filepath.Join(dir, "huge.md")
	require.NoError(t, os.WriteFile(huge, nil, 0o600))
	require.NoError(t, os.Truncate(huge, int64(hardwrap.SizeLimit)+1))
	link := filepath.Join(dir, "link.md")
	require.NoError(t, os.Symlink(huge, link))

	_, err := readFile(link)

	assert.ErrorIs(t, err, goyze.ErrTooLarge)
}

// TestAnOversizeDocumentIsReportedByBothEntryPoints pins that the walk and the
// named path agree, and that neither passes over it in silence — a document too
// large to analyze is exactly where unchecked prose hides.
func TestAnOversizeDocumentIsReportedByBothEntryPoints(t *testing.T) {
	dir := t.TempDir()
	huge := filepath.Join(dir, "huge.md")
	require.NoError(t, os.WriteFile(huge, nil, 0o600))
	require.NoError(t, os.Truncate(huge, int64(hardwrap.SizeLimit)+1))

	for name, args := range map[string][]string{"walked": {dir}, "named": {huge}} {
		buf := swapStdout(t)
		require.Equal(t, 0, run(args), name)
		assert.Contains(t, buf.String(), "huge.md", "%s: reported, not dropped", name)
		assert.Contains(t, buf.String(), "too large", name)
	}
}
