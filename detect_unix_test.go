//go:build unix

package terminal

import (
	"strings"
	"testing"
)

// A text console gets 16 colors whatever COLORTERM says, NO_COLOR turns
// color off unless the caller names a mode, and TERM=dumb is refused
func TestDetectionFollowsTheTerminal(t *testing.T) {
	t.Setenv("TERM", "linux")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("NO_COLOR", "")
	if got := New().ColorMode(); got != ColorMode16 {
		t.Errorf("text console: mode %d", got)
	}
	t.Setenv("NO_COLOR", "1")
	if got, named := New().ColorMode(), New(ColorMode256).ColorMode(); got != ColorModeNone || named != ColorMode256 {
		t.Errorf("NO_COLOR: mode %d, named %d", got, named)
	}
	t.Setenv("TERM", "dumb")
	if err := newBackend().Init(); err == nil || !strings.Contains(err.Error(), "dumb") {
		t.Errorf("TERM=dumb: %v", err)
	}
}
