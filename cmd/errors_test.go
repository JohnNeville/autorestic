package cmd

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorsFound(t *testing.T) {
	if errorsFound(nil) != nil {
		t.Error("no errors should give nil")
	}
	err := errorsFound([]error{errors.New("root@nas:\nrepository is locked"), errors.New("second")})
	for _, want := range []string{"2 errors were found:", "root@nas:", "repository is locked", "second"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q missing %q", err, want)
		}
	}
}
