// Package journal sends messages to the systemd journal over journald's
// native protocol, so they keep their priority and structured fields even when
// the process's stdout/stderr go to a log file.
package journal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type Priority int

// syslog(3) levels.
const (
	Err     Priority = 3
	Warning Priority = 4
	Notice  Priority = 5
	Info    Priority = 6
)

const Identifier = "autorestic"

// journald rejects datagrams over its socket buffer size (typically 208 KiB+).
// Messages are short notices plus at most some restic output, so cap them well
// below that instead of handling the memfd fallback.
const maxMessage = 64 * 1024

// Seams for tests.
var (
	socketPath = "/run/systemd/journal/socket"
	getenv     = os.Getenv
	fstat      = syscall.Fstat
)

var ErrUnavailable = errors.New("systemd journal socket not available")

// Available reports whether journald's native socket exists.
func Available() bool {
	_, err := os.Stat(socketPath)
	return err == nil
}

// Send writes one journal entry with SYSLOG_IDENTIFIER=autorestic. Extra field
// names must be upper case letters, digits and underscores; others are dropped.
func Send(message string, priority Priority, fields map[string]string) error {
	if !Available() {
		return ErrUnavailable
	}
	conn, err := net.DialUnix("unixgram", nil, &net.UnixAddr{Name: socketPath, Net: "unixgram"})
	if err != nil {
		return err
	}
	defer conn.Close()
	_, err = conn.Write(encode(message, priority, fields))
	return err
}

func encode(message string, priority Priority, fields map[string]string) []byte {
	if len(message) > maxMessage {
		message = message[:maxMessage] + "\n[truncated]"
	}
	var b bytes.Buffer
	writeField(&b, "MESSAGE", message)
	writeField(&b, "PRIORITY", strconv.Itoa(int(priority)))
	writeField(&b, "SYSLOG_IDENTIFIER", Identifier)
	for k, v := range fields {
		if validName(k) && v != "" {
			writeField(&b, k, v)
		}
	}
	return b.Bytes()
}

// writeField uses the plain KEY=VALUE form, or for values containing a newline
// the binary form: KEY, newline, little-endian uint64 length, value, newline.
func writeField(b *bytes.Buffer, key, value string) {
	if !strings.Contains(value, "\n") {
		b.WriteString(key + "=" + value + "\n")
		return
	}
	b.WriteString(key + "\n")
	binary.Write(b, binary.LittleEndian, uint64(len(value)))
	b.WriteString(value + "\n")
}

func validName(k string) bool {
	if k == "" || k[0] == '_' || (k[0] >= '0' && k[0] <= '9') {
		return false
	}
	for _, r := range k {
		if !(r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_') {
			return false
		}
	}
	return true
}

// IsJournalStream reports whether file descriptor fd is the journal stream
// systemd connected it to (JOURNAL_STREAM holds that stream's "device:inode"),
// i.e. whether plain writes to it already end up in the journal.
func IsJournalStream(fd int) bool {
	want := getenv("JOURNAL_STREAM")
	if want == "" {
		return false
	}
	var st syscall.Stat_t
	if err := fstat(fd, &st); err != nil {
		return false
	}
	return want == strconv.FormatUint(uint64(st.Dev), 10)+":"+strconv.FormatUint(uint64(st.Ino), 10)
}
