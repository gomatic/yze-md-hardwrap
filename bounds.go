package hardwrap

// How much of a RUN a report may carry, and what it says when it carries less
// than it found. The size limit bounds one document and the per-document limit
// bounds its findings; neither bounds a tree of them.

// reportLimit bounds how many findings ONE RUN carries. A tree whose documents
// are each at their own limit is a report an order of magnitude larger than the
// input that produced it, which is the same failure the per-document limit
// exists to prevent, reached by a route it does not cover. A run with this many
// findings is one problem too, and the true count is still named.
const reportLimit findingCount = 10_000

// runTruncationMessage formats the finding that stands for the rest of a run.
const runTruncationMessage = "%d hard-wrapped blocks across this run, of which %d are reported; findings from %s " +
	"onward are omitted, because a tree with this many is one problem rather than that many"
