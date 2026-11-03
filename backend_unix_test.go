//go:build linux

package terminal

import (
	"testing"
	"time"

	"golang.org/x/term"
)

// A read waits on the terminal with no limit until input comes or Wake is
// called, a Wake made before the read ends it at once, and a wait it is given
// passes: an idle terminal takes no wakeup
func TestUnixReadWaitsForInputOrWake(t *testing.T) {
	master, tty := openPty(t)
	if _, err := term.MakeRaw(int(tty.Fd())); err != nil {
		t.Fatal(err)
	}
	b := &unixBackend{inFd: int(tty.Fd()), wake: [2]int{-1, -1}}
	if err := b.openWake(); err != nil {
		t.Fatal(err)
	}
	defer b.closeWake()
	read := func(wait time.Duration) (string, time.Duration) {
		t.Helper()
		start := time.Now()
		data, err := b.Read(wait)
		if err != nil {
			t.Fatal(err)
		}
		return string(data), time.Since(start)
	}

	if got, took := read(30 * time.Millisecond); got != "" || took < 30*time.Millisecond {
		t.Errorf("a 30 ms wait: %q after %v", got, took)
	}
	time.AfterFunc(60*time.Millisecond, b.Wake)
	if got, took := read(-1); got != "" || took < 50*time.Millisecond {
		t.Errorf("no limit, woken at 60 ms: %q after %v", got, took)
	}
	time.AfterFunc(60*time.Millisecond, func() { master.Write([]byte("q")) })
	if got, took := read(-1); got != "q" || took < 50*time.Millisecond {
		t.Errorf("no limit, typed at 60 ms: %q after %v", got, took)
	}
	b.Wake()
	if got, took := read(-1); got != "" || took > time.Second {
		t.Errorf("woken before the read: %q after %v", got, took)
	}
}
