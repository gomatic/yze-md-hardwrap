// Command yze-md-hardwrap reports prose whose source spans more lines than it
// renders — a newline inside a paragraph, list item, blockquote or heading that
// no renderer shows — emitting the lean stickler-json report the stickler runner
// consumes.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	goyze "github.com/gomatic/go-yze"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// Injected collaborators, so the command is testable without real I/O.
//
// The filesystem is ONE value rather than a scatter of package variables, so
// there is a single seam every path is read through — the size bound, the
// symlinked walk root and the unreadable directory are all arrangeable from a
// test, which is what makes those branches testable at all.
var (
	osExit           = os.Exit
	files            = goyze.OSFileSystem()
	stdout io.Writer = os.Stdout
)

func main() { osExit(run(os.Args[1:])) }

// run expands the arguments to documents, runs the analyzer, and emits the
// report.
func run(args []string) int {
	if err := report(args); err != nil {
		return fail(err)
	}
	return 0
}

// report is the run itself, as an ERROR rather than an exit code. run answers
// the process, which cannot be matched: with the refusal only ever reaching an
// int and a line of stderr, this command's sentinel could be swapped for any
// other with the whole suite green, so the failure the sentinel exists to name
// would have nothing behind it.
func report(args []string) error {
	if len(args) == 0 {
		// Being given nothing is an error, not a clean pass. A runner whose
		// root placeholder expands to nothing would otherwise green the gate
		// over a repository no analyzer ever looked at.
		return hardwrap.ErrNoPaths.With(nil)
	}
	docs := configuredDocuments()
	found, err := discovery(docs).Expand(args)
	if err != nil {
		return err
	}
	// Report cannot fail: an unreadable or unparseable document becomes a
	// finding against that file rather than the run's error.
	out := hardwrap.Report(readFile, found.Files, docs)
	out.Diagnostics = append(hardwrap.Unreadable(found.Unreadable), out.Diagnostics...)
	return json.NewEncoder(stdout).Encode(out)
}

// fail prints err to stderr and returns the failure exit code.
func fail(err error) int {
	_, _ = fmt.Fprintln(os.Stderr, "yze-md-hardwrap:", err)
	return 1
}
