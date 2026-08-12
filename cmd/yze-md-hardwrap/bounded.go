package main

// Bounding what is read. The limit belongs at the single place every path is
// read — not in the walk — because a bound that guards one entry point and not
// the other is not a bound: a file NAMED on the command line never passes the
// walk at all, and one that is only stat'ed is judged by the size of a symlink
// rather than of the file behind it.

import (
	goyze "github.com/gomatic/go-yze"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// readFile reads a document, bounded, at the ONE place every path is read.
var readFile = func(path string) ([]byte, error) {
	return goyze.BoundedRead(files, goyze.FilePath(path), hardwrap.SizeLimit)
}
