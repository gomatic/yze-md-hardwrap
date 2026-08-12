package main

// Where this command reads its configuration: the process environment, at the
// one boundary where ambient state is visible, handed to the library as a value.
// The library takes the document set as a parameter and never reaches for the
// environment itself — a library that does makes every caller, and every test,
// depend on what happened to be exported.

import (
	"os"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// lookupEnv reads one environment variable. It is a seam so a test can supply a
// value without exporting one, and so this command's ONE ambient input can be
// neutralised for the tests that are not about it.
var lookupEnv = os.Getenv

// configuredDocuments is the set of file kinds this run reads: markdown always,
// plus whatever the repository asked for.
func configuredDocuments() hardwrap.Documents {
	return hardwrap.ConfiguredDocuments(lookupEnv)
}
