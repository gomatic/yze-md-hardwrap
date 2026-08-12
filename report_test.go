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

// crowded is a set of documents holding `each` findings apiece, enough of them
// to press against the run's limit.
func crowded(count, each int) (map[string]string, []string) {
	files := make([]string, 0, count)
	contents := map[string]string{}
	for i := range count {
		name := "d" + strconv.Itoa(i) + ".md"
		files = append(files, name)
		contents[name] = strings.Repeat(wrappedDocument+"\n", each)
	}
	return contents, files
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

	report := hardwrap.Report(read, []string{"a.md", "b.md", "c.md"}, nil, hardwrap.DefaultSettings())

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

	assert.Empty(t, hardwrap.Report(read, files, nil, hardwrap.DefaultSettings()).Diagnostics)
	configured, err := hardwrap.ConfiguredSettings(environment(map[string]string{documentsVariable: ".txt"}))
	require.NoError(t, err)
	assert.Len(t, hardwrap.Report(read, files, nil, configured).Diagnostics, 1)
}

// TestReportJudgesLineEndingsTheWayTheRunWasConfigured pins that the OTHER
// setting reaches the aggregation too, and pins which way round the two runs
// are: the configured run reports less than the default one, never more.
func TestReportJudgesLineEndingsTheWayTheRunWasConfigured(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{"notes.md": "a paragraph that ends a line the format renders  \nand carries on\n"})
	files := []string{"notes.md"}

	assert.Len(t, hardwrap.Report(read, files, nil, hardwrap.DefaultSettings()).Diagnostics, 1,
		"the default run judges every line ending")
	configured, err := hardwrap.ConfiguredSettings(environment(map[string]string{breaksVariable: "authored"}))
	require.NoError(t, err)
	assert.Empty(t, hardwrap.Report(read, files, nil, configured).Diagnostics,
		"and the configured run leaves the ones the format renders")
}

// TestReportContainsAReadFailureToItsOwnFile pins that a document the gate
// cannot open is reported rather than fatal — and that the finding carries the
// CAUSE, so a locked file is distinguishable from a malformed one.
func TestReportContainsAReadFailureToItsOwnFile(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{"notes.md": wrappedDocument})

	report := hardwrap.Report(read, []string{"locked.md", "notes.md"}, nil, hardwrap.DefaultSettings())

	messages := map[string]string{}
	for _, diag := range report.Diagnostics {
		messages[diag.Path] += diag.Message
	}
	assert.Contains(t, messages["locked.md"], "cannot be analyzed as a document")
	assert.Contains(t, messages["locked.md"], hardwrap.ErrReadFile.Error())
	assert.Contains(t, messages["locked.md"], errUnreadable.Error(), "the cause the filesystem gave")
	assert.Contains(t, messages["notes.md"], wrapMarker, "its neighbour keeps its finding")
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

	report := hardwrap.Report(read, []string{"blob.md", "notes.md"}, nil, hardwrap.DefaultSettings())

	require.Len(t, report.Diagnostics, 2)
	assert.Contains(t, report.Diagnostics[0].Message, "cannot be analyzed as a document")
	assert.Contains(t, report.Diagnostics[0].Message, hardwrap.ErrNotText.Error())
	assert.Equal(t, "notes.md", report.Diagnostics[1].Path, "its neighbour keeps its finding")
}

// TestReportOfNoFilesIsAnEmptyReport pins the trivial case explicitly.
func TestReportOfNoFilesIsAnEmptyReport(t *testing.T) {
	t.Parallel()

	assert.Empty(t, hardwrap.Report(reader(nil), nil, nil, hardwrap.DefaultSettings()).Diagnostics)
}

// TestUnreadablePathsAreReportedRatherThanSkipped pins that a path the walk
// could not read reaches the report, and does so ALONGSIDE the documents' own
// findings. A path the gate cannot open is exactly where an unchecked document
// would hide.
func TestUnreadablePathsAreReportedRatherThanSkipped(t *testing.T) {
	t.Parallel()

	read := reader(map[string]string{"notes.md": wrappedDocument})

	report := hardwrap.Report(read, []string{"notes.md"}, []string{"locked", "gone"}, hardwrap.DefaultSettings())

	require.Len(t, report.Diagnostics, 3)
	assert.Equal(t, "locked", report.Diagnostics[0].Path)
	assert.Contains(t, report.Diagnostics[0].Message, hardwrap.ErrReadFile.Error())
	assert.Equal(t, hardwrap.Rule, report.Diagnostics[1].Rule)
	assert.Equal(t, "notes.md", report.Diagnostics[2].Path, "and the documents keep their findings")
}

// TestUnreadablePathsPassThroughTheRunsOwnLimit pins that they are bounded like
// everything else. Reported BESIDE the run rather than through it, a tree of
// unreadable directories was the one unbounded path left in a bounded report.
func TestUnreadablePathsPassThroughTheRunsOwnLimit(t *testing.T) {
	t.Parallel()

	locked := make([]string, 0, 10_100)
	for i := range 10_100 {
		locked = append(locked, "locked"+strconv.Itoa(i))
	}

	report := hardwrap.Report(reader(nil), nil, locked, hardwrap.DefaultSettings())

	require.Len(t, report.Diagnostics, 10_001, "the run limit, plus the notice that stands for the rest")
	assert.Contains(t, report.Diagnostics[10_000].Message, "10100 hard-wrapped blocks across this run")
}

// TestARunExactlyAtItsLimitCarriesNoTruncationNotice pins the boundary itself.
// One finding either side of the limit is what tells a `>` from a `>=`, and the
// notice must appear only when something really was dropped.
func TestARunExactlyAtItsLimitCarriesNoTruncationNotice(t *testing.T) {
	t.Parallel()

	contents, files := crowded(10, 1000)

	report := hardwrap.Report(reader(contents), files, nil, hardwrap.DefaultSettings())

	require.Len(t, report.Diagnostics, 10_000, "exactly the limit fits, and nothing stands for a remainder")
	for _, diag := range report.Diagnostics {
		assert.Contains(t, diag.Message, wrapMarker, "no truncation notice among them")
	}
}

// TestTheRunLimitBoundsTheReportAndNeverLosesTheCount pins the bound the
// per-document limit does not provide: a document limit bounds a document, not a
// tree of them. The truncation notice carries the TRUE total, which is the half
// a summed slice would get wrong.
func TestTheRunLimitBoundsTheReportAndNeverLosesTheCount(t *testing.T) {
	t.Parallel()

	contents, files := crowded(30, 500)

	report := hardwrap.Report(reader(contents), files, nil, hardwrap.DefaultSettings())

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

	contents, files := crowded(40, 500)

	report := hardwrap.Report(reader(contents), files, nil, hardwrap.DefaultSettings())

	assert.Contains(t, report.Diagnostics[10_000].Message, "20000 hard-wrapped blocks across this run",
		"ten more files were counted after collection stopped")
}

// TestEveryDiagnosticCarriesTheRuleIdentity pins the contract the runner
// consumes: a diagnostic without a rule id cannot be softened, baselined or
// attributed, and the shared report refuses one.
func TestEveryDiagnosticCarriesTheRuleIdentity(t *testing.T) {
	t.Parallel()

	report := hardwrap.Report(reader(map[string]string{"a.md": wrappedDocument}), []string{"a.md", "gone.md"},
		[]string{"locked"}, hardwrap.DefaultSettings())

	encoded, err := goyze.MarshalReport(report)
	require.NoError(t, err)
	parsed, err := goyze.UnmarshalReport(encoded)
	require.NoError(t, err, "a report the runner refuses is worse than no report")
	require.Len(t, parsed.Diagnostics, 3)
	for _, diag := range parsed.Diagnostics {
		assert.Equal(t, hardwrap.Rule, diag.Rule)
		assert.Equal(t, hardwrap.Tool, diag.Tool)
		assert.Positive(t, diag.Line)
		assert.Positive(t, diag.Col)
	}
}
