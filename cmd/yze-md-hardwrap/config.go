package main

// Where this command reads its configuration: the process environment, at the
// one boundary where ambient state is visible, handed to the library as a value.
// The library takes the run's settings as a parameter and never reaches for the
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

// configuredSettings is what this run was configured with: the file kinds it
// reads, and how it judges a line ending.
//
// A configuration this command cannot apply FAILS the run rather than being
// dropped on the way past. An analyzer that silently ignores a line of a
// repository's own config reports the same as an unconfigured one, and the
// repository has no way to learn which of the two it got.
func configuredSettings() (hardwrap.Settings, error) {
	return hardwrap.ConfiguredSettings(lookupEnv)
}
