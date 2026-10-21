//go:build linux

package terminal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// The guard ends armed with the last arm's mode and termios when the arms
// outnumber the disarms, and disarmed otherwise
func TestGuardFollowsArmsToTheEnd(t *testing.T) {
	arm := func(mode byte, iflag uint32) []byte {
		tio := unix.Termios{Iflag: iflag}
		return append([]byte{guardArm, mode}, termiosBytes(&tio)...)
	}
	cases := []struct {
		name  string
		msgs  [][]byte
		armed bool
		mode  int
		iflag uint32
	}{
		{"never armed", nil, false, 0, 0},
		{"armed", [][]byte{arm(3, 7)}, true, 3, 7},
		{"disarmed", [][]byte{arm(3, 7), {guardDisarm}}, false, 0, 0},
		{"armed again", [][]byte{arm(3, 7), {guardDisarm}, arm(1, 9)}, true, 1, 9},
		{"cut off arming", [][]byte{{guardArm, 3, 1, 2}}, false, 0, 0},
	}
	for _, c := range cases {
		armed, mode, tio := guardWait(bytes.NewReader(bytes.Join(c.msgs, nil)))
		if armed != c.armed || armed && (mode != c.mode || tio.Iflag != c.iflag) {
			t.Errorf("%s: armed %v mode %d iflag %d, want %v %d %d", c.name, armed, mode, tio.Iflag, c.armed, c.mode, c.iflag)
		}
	}
}

// openPty opens a pty pair, the far end standing in for the console
func openPty(t *testing.T) (master, tty *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skip("no pty:", err)
	}
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	tty, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tty.Close(); master.Close() })
	return master, tty
}

// guardGame is a game for TestGuardRestoresWhenTheGameIsKilled: it guards the
// pty it is handed, arms the guard with the pty's termios as cooked, makes
// the pty raw, says so and waits to be killed
func TestGuardGame(t *testing.T) {
	path := os.Getenv("TERMINAL_TEST_GUARD_GAME")
	if path == "" {
		t.Skip("run by TestGuardRestoresWhenTheGameIsKilled")
	}
	tty, err := os.OpenFile(path, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	cooked, err := unix.IoctlGetTermios(int(tty.Fd()), tcgets)
	if err != nil {
		t.Fatal(err)
	}
	g, err := startGuard(tty)
	if err != nil {
		t.Fatal(err)
	}
	if !g.arm(kUnicode, cooked) {
		t.Fatal("arming failed")
	}
	raw := *cooked
	raw.Lflag &^= unix.ICANON | unix.ECHO | unix.ISIG
	if err := unix.IoctlSetTermios(int(tty.Fd()), tcsets, &raw); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("TERMINAL_TEST_GUARD_DISARM") != "" {
		g.disarm()
	}
	fmt.Println("raw")
	time.Sleep(time.Minute)
}

// A game killed with its console raw leaves the guard to restore it: the
// termios the game found, and the cursor shown, with no reset that would
// clear the game's last words; a game that disarmed first leaves it as it was
func TestGuardRestoresWhenTheGameIsKilled(t *testing.T) {
	for _, disarm := range []bool{false, true} {
		master, tty := openPty(t)
		cooked, err := unix.IoctlGetTermios(int(tty.Fd()), tcgets)
		if err != nil {
			t.Fatal(err)
		}
		game := exec.Command(os.Args[0], "-test.run=^TestGuardGame$", "-test.v")
		game.Env = append(os.Environ(), "TERMINAL_TEST_GUARD_GAME="+tty.Name())
		if disarm {
			game.Env = append(game.Env, "TERMINAL_TEST_GUARD_DISARM=1")
		}
		stdout, err := game.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := game.Start(); err != nil {
			t.Fatal(err)
		}
		lines := bufio.NewScanner(stdout)
		for lines.Scan() && lines.Text() != "raw" {
		}
		game.Process.Kill()
		game.Wait()

		restored := func() bool {
			now, err := unix.IoctlGetTermios(int(tty.Fd()), tcgets)
			return err == nil && now.Lflag == cooked.Lflag
		}
		if !disarm {
			waitFor(t, "the termios restored", restored)
			out := readPending(t, master)
			if !strings.Contains(out, string(csiCursorShow)) || strings.Contains(out, string(csiRIS)) {
				t.Errorf("guard wrote %q, want the cursor shown and no reset", out)
			}
			continue
		}
		time.Sleep(200 * time.Millisecond)
		if restored() {
			t.Error("a disarmed guard restored the termios")
		}
		if out := readPending(t, master); out != "" {
			t.Errorf("a disarmed guard wrote %q", out)
		}
	}
}

// readPending is what the pty's master has to read now
func readPending(t *testing.T, master *os.File) string {
	t.Helper()
	fds := []unix.PollFd{{Fd: int32(master.Fd()), Events: unix.POLLIN}}
	var out []byte
	buf := make([]byte, 4096)
	for {
		if n, err := unix.Poll(fds, 100); err != nil || n == 0 {
			return string(out)
		}
		n, err := master.Read(buf)
		if err != nil && err != io.EOF || n == 0 {
			return string(out)
		}
		out = append(out, buf[:n]...)
	}
}
