package internal

import (
	"fmt"
	"os"

	"github.com/JohnNeville/autorestic/internal/colors"
	"github.com/JohnNeville/autorestic/internal/flags"
	"github.com/JohnNeville/autorestic/internal/journal"
)

// Seam for tests.
var journalSend = journal.Send

// Announce prints a message that must not get lost in a long backup log.
//
// CI mode assumes autorestic runs from a systemd service or timer, where stdout
// often goes to a log file nobody reads, so the message is also sent to the
// systemd journal with its priority and fields (SYSLOG_IDENTIFIER=autorestic).
// Without a journal it goes to stderr instead. If stdout already is the
// journal, it is only sent once.
func Announce(priority journal.Priority, fields map[string]string, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if !flags.CI {
		printAnnouncement(priority, msg)
		return
	}
	sent := journalSend(msg, priority, fields) == nil
	if !sent || !journal.IsJournalStream(1) {
		printAnnouncement(priority, msg)
	}
	if !sent && !journal.IsJournalStream(2) {
		fmt.Fprintln(os.Stderr, msg)
	}
}

func printAnnouncement(priority journal.Priority, msg string) {
	if priority <= journal.Warning {
		colors.Error.Println(msg)
	} else {
		colors.Secondary.Println(msg)
	}
}
