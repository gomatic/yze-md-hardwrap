package hardwrap_test

// Aggregating a run. The property every case here defends is the same one: a
// file the gate cannot read must never take another file's findings down with
// it, and a run that reports less than it found must say so.

import (
	"strconv"
	"strings"
	"testing"

	errs "github.com/gomatic/go-error"
	goyze "github.com/gomatic/go-yze"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// errUnreadable stands in for whatever the filesystem refuses with.
const errUnreadable errs.Const = "unreadable"

// wrappedDocument is the smallest document carrying one finding.
const wrappedDocument = "a paragraph that is\nwrapped\n"

// reader serves file contents from a map, refusing anything absent.
func reader(files map[string]string) hardwrap.FileReader {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, errUnreadable
		}
		return []byte(data), nil
	}
}

// TestReportAggregatesEveryDocumentsFindings pins that a run over several files
// yields all their findings, each naming the file it came from.
func TestReportAggregatesEveryDocumentsFindings(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{
		"a.md": wrappedDocument,
		"b.md": "one line, however long\n",
		"c.md": wrappedDocument,
	})

	report := hardwrap.Report(read, []string{"a.md", "b.md", "c.md"}, hardwrap.DefaultDocuments())

	require.Len(t, report.Diagnostics, 2)
	assert.Equal(t, "a.md", report.Diagnostics[0].Path)
	assert.Equal(t, "c.md", report.Diagnostics[1].Path)
}

// TestReportReadsWhatTheRunWasConfiguredToRead pins that the document set
// reaches the aggregation too: a run told to read `.txt` reads it here, and a
// run that was not told does not.
func TestReportReadsWhatTheRunWasConfiguredToRead(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{"notes.txt": wrappedDocument})
	files := []string{"notes.txt"}

	assert.Empty(t, hardwrap.Report(read, files, hardwrap.DefaultDocuments()).Diagnostics)
	configured := hardwrap.ConfiguredDocuments(func(string) string { return ".txt" })
	assert.Len(t, hardwrap.Report(read, files, configured).Diagnostics, 1)
}

// TestReportContainsAReadFailureToItsOwnFile pins that a document the gate
// cannot open is reported rather than fatal — and that the finding carries the
// CAUSE, so a locked file is distinguishable from a malformed one.
func TestReportContainsAReadFailureToItsOwnFile(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{"notes.md": wrappedDocument})

	report := hardwrap.Report(read, []string{"locked.md", "notes.md"}, hardwrap.DefaultDocuments())

	messages := map[string]string{}
	for _, diag := range report.Diagnostics {
		messages[diag.Path] += diag.Message
	}
	assert.Contains(t, messages["locked.md"], "cannot be analyzed as a document")
	assert.Contains(t, messages["locked.md"], hardwrap.ErrReadFile.Error())
	assert.Contains(t, messages["locked.md"], errUnreadable.Error(), "the cause the filesystem gave")
	assert.Contains(t, messages["notes.md"], "hard-wrapped", "its neighbour keeps its finding")
}

// TestReportContainsAnUnreadableDocumentToItsOwnFile pins the containment: a
// binary blob mis-claimed by discovery becomes one finding against that file
// and the run continues, so it cannot silence its neighbours.
func TestReportContainsAnUnreadableDocumentToItsOwnFile(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{
		"blob.md":  string([]byte{0xff, 0xfe, 0x00}),
		"notes.md": wrappedDocument,
	})

	report := hardwrap.Report(read, []string{"blob.md", "notes.md"}, hardwrap.DefaultDocuments())

	require.Len(t, report.Diagnostics, 2)
	assert.Contains(t, report.Diagnostics[0].Message, "cannot be analyzed as a document")
	assert.Contains(t, report.Diagnostics[0].Message, hardwrap.ErrNotText.Error())
	assert.Equal(t, "notes.md", report.Diagnostics[1].Path, "its neighbour keeps its finding")
}

// TestReportOfNoFilesIsAnEmptyReport pins the trivial case explicitly.
func TestReportOfNoFilesIsAnEmptyReport(t *testing.T) {
	t.Parallel()

	assert.Empty(t, hardwrap.Report(reader(nil), nil, hardwrap.DefaultDocuments()).Diagnostics)
}

// TestUnreadablePathsAreReportedRatherThanSkipped pins that a path the walk
// could not read reaches the report. A path the gate cannot open is exactly
// where an unchecked document would hide.
func TestUnreadablePathsAreReportedRatherThanSkipped(t *testing.T) {
	t.Parallel()

	diags := hardwrap.Unreadable([]string{"locked", "gone"})

	require.Len(t, diags, 2)
	assert.Equal(t, "locked", diags[0].Path)
	assert.Contains(t, diags[0].Message, hardwrap.ErrReadFile.Error())
	assert.Equal(t, hardwrap.Rule, diags[1].Rule)
}

// TestUnreadableOfNothingIsNothing pins that a clean walk contributes no
// findings of its own.
func TestUnreadableOfNothingIsNothing(t *testing.T) {
	t.Parallel()

	assert.Empty(t, hardwrap.Unreadable(nil))
}

// TestTheRunLimitBoundsTheReportAndNeverLosesTheCount pins the bound the
// per-document limit does not provide: a document limit bounds a document, not a
// tree of them. The truncation notice carries the TRUE total, which is the half
// a summed slice would get wrong.
func TestTheRunLimitBoundsTheReportAndNeverLosesTheCount(t *testing.T) {
	t.Parallel()

	files := make([]string, 0, 30)
	contents := map[string]string{}
	for i := range 30 {
		name := "d" + strconv.Itoa(i) + ".md"
		files = append(files, name)
		contents[name] = strings.Repeat(wrappedDocument+"\n", 500)
	}

	report := hardwrap.Report(reader(contents), files, hardwrap.DefaultDocuments())

	require.Len(t, report.Diagnostics, 10_001, "the run limit, plus the notice that stands for the rest")
	last := report.Diagnostics[10_000]
	assert.Contains(t, last.Message, "15000 hard-wrapped blocks across this run",
		"the true total, not the number collected")
	assert.Contains(t, last.Message, "of which 10000 are reported")
	assert.NotEmpty(t, last.Path, "attributed to the file collection stopped at, never to no file")
}

// TestARunPastItsLimitStillCountsWhatItStoppedCollecting pins that counting and
// collecting are separate: the files after the limit contribute to the total
// even though none of their findings are carried.
func TestARunPastItsLimitStillCountsWhatItStoppedCollecting(t *testing.T) {
	t.Parallel()

	files := make([]string, 0, 40)
	contents := map[string]string{}
	for i := range 40 {
		name := "d" + strconv.Itoa(i) + ".md"
		files = append(files, name)
		contents[name] = strings.Repeat(wrappedDocument+"\n", 500)
	}

	report := hardwrap.Report(reader(contents), files, hardwrap.DefaultDocuments())

	assert.Contains(t, report.Diagnostics[10_000].Message, "20000 hard-wrapped blocks across this run",
		"ten more files were counted after collection stopped")
}

// TestEveryDiagnosticCarriesTheRuleIdentity pins the contract the runner
// consumes: a diagnostic without a rule id cannot be softened, baselined or
// attributed, and the shared report refuses one.
func TestEveryDiagnosticCarriesTheRuleIdentity(t *testing.T) {
	t.Parallel()

	report := hardwrap.Report(reader(map[string]string{"a.md": wrappedDocument}), []string{"a.md", "gone.md"},
		hardwrap.DefaultDocuments())

	encoded, err := goyze.MarshalReport(report)
	require.NoError(t, err)
	parsed, err := goyze.UnmarshalReport(encoded)
	require.NoError(t, err, "a report the runner refuses is worse than no report")
	require.Len(t, parsed.Diagnostics, 2)
	for _, diag := range parsed.Diagnostics {
		assert.Equal(t, hardwrap.Rule, diag.Rule)
		assert.Equal(t, hardwrap.Tool, diag.Tool)
		assert.Positive(t, diag.Line)
		assert.Positive(t, diag.Col)
	}
}
