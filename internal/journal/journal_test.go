package journal

import (
	"bytes"
	"encoding/binary"
	"net"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestEncodePlainAndBinaryFields(t *testing.T) {
	got := encode("line one\nline two", Err, map[string]string{
		"AUTORESTIC_BACKEND": "nas",
		"lower":              "dropped",
		"_TRUSTED":           "dropped",
		"EMPTY":              "",
	})
	var want bytes.Buffer
	want.WriteString("MESSAGE\n")
	binary.Write(&want, binary.LittleEndian, uint64(len("line one\nline two")))
	want.WriteString("line one\nline two\n")
	want.WriteString("PRIORITY=3\nSYSLOG_IDENTIFIER=autorestic\nAUTORESTIC_BACKEND=nas\n")
	if !bytes.Equal(got, want.Bytes()) {
		t.Errorf("encode:\n got %q\nwant %q", got, want.Bytes())
	}
}

func TestEncodeTruncatesHugeMessages(t *testing.T) {
	got := encode(strings.Repeat("x", maxMessage+10), Info, nil)
	if len(got) > maxMessage+200 {
		t.Errorf("message not truncated: %d bytes", len(got))
	}
}

func TestSendToSocket(t *testing.T) {
	dir := t.TempDir()
	old := socketPath
	socketPath = filepath.Join(dir, "socket")
	defer func() { socketPath = old }()

	l, err := net.ListenUnixgram("unixgram", &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	if err := Send("hello", Notice, map[string]string{"AUTORESTIC_BACKEND": "b"}); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	l.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := l.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(buf[:n]), "MESSAGE=hello\nPRIORITY=5\nSYSLOG_IDENTIFIER=autorestic\nAUTORESTIC_BACKEND=b\n"; got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

func TestSendWithoutSocket(t *testing.T) {
	old := socketPath
	socketPath = filepath.Join(t.TempDir(), "missing")
	defer func() { socketPath = old }()
	if err := Send("x", Info, nil); err != ErrUnavailable {
		t.Errorf("got %v, want ErrUnavailable", err)
	}
}

func TestIsJournalStream(t *testing.T) {
	oldEnv, oldStat := getenv, fstat
	defer func() { getenv, fstat = oldEnv, oldStat }()
	fstat = func(fd int, st *syscall.Stat_t) error { st.Dev, st.Ino = 25, 4096; return nil }

	for env, want := range map[string]bool{"25:4096": true, "25:4097": false, "": false} {
		getenv = func(string) string { return env }
		if got := IsJournalStream(2); got != want {
			t.Errorf("JOURNAL_STREAM=%q: got %v want %v", env, got, want)
		}
	}
}
