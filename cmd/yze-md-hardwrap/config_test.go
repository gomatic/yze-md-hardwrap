package main

// The one ambient input this command has, and the seam it is read through.

import (
	"testing"

	"github.com/stretchr/testify/assert"

	hardwrap "github.com/gomatic/yze-md-hardwrap"
)

// TestConfigurationIsReadThroughTheSeam pins that the environment reaches the
// document set — and that it does so through an injected lookup, so a test sees
// a configuration change without exporting one into the process.
func TestConfigurationIsReadThroughTheSeam(t *testing.T) {
	original := lookupEnv
	lookupEnv = func(name string) string {
		if name == "YZE_HARDWRAP_DOCUMENTS" {
			return ".txt"
		}
		return ""
	}
	t.Cleanup(func() { lookupEnv = original })

	docs := configuredDocuments()

	assert.True(t, docs.Reads("notes.txt"), "the configured kind is read")
	assert.True(t, docs.Reads("README.md"), "and markdown still is")
}

// TestAnUnconfiguredRunReadsMarkdown pins the default at the command's edge: an
// unset variable is the default set, not an empty one that reads nothing.
func TestAnUnconfiguredRunReadsMarkdown(t *testing.T) {
	assert.Equal(t, hardwrap.DefaultDocuments(), configuredDocuments())
}
