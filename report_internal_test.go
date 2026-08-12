package hardwrap

// The one error this package builds that never leaves it as an error. It is
// rendered into a finding's message, so nothing outside can match it — and a
// sentinel nothing can match could be swapped for any other with the whole
// suite green.

import (
	"testing"

	errs "github.com/gomatic/go-error"
	"github.com/stretchr/testify/assert"
)

// errLocked stands in for whatever the filesystem refuses with.
const errLocked errs.Const = "locked"

// TestAReadFailureCarriesBothItsSentinelAndItsCause pins what the finding for
// an unopenable path is built from: the rule's own sentinel, so a caller can
// match the CONDITION, and the filesystem's cause, so a locked file is
// distinguishable from a malformed one.
func TestAReadFailureCarriesBothItsSentinelAndItsCause(t *testing.T) {
	t.Parallel()

	err := readFailure("locked.md", errLocked)

	assert.ErrorIs(t, err, ErrReadFile)
	assert.ErrorIs(t, err, errLocked)
	assert.Contains(t, err.Error(), "locked.md", "and the path it happened to")
}

// TestAReadFailureWithNoCauseIsStillTheSentinel pins the walk's case: a path it
// could not read hands back no cause of its own, and the finding must still be
// matchable as this rule's read failure.
func TestAReadFailureWithNoCauseIsStillTheSentinel(t *testing.T) {
	t.Parallel()

	assert.ErrorIs(t, readFailure("locked", nil), ErrReadFile)
}
