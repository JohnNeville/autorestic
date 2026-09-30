package internal

import "testing"

func TestResticLockedRe(t *testing.T) {
	cases := map[string][]string{
		"unable to create lock in backend: repository is already locked exclusively by PID 3841541 on paulrevere by root (UID 0, GID 0)\nlock was created at ...": {"3841541", "paulrevere"},
		"repository is already locked by PID 12 on host-a.lan by john (UID 1000, GID 1000)":                                                                       {"12", "host-a.lan"},
		"Fatal: wrong password or no key found": nil,
	}
	for in, want := range cases {
		m := resticLockedRe.FindStringSubmatch(in)
		if want == nil {
			if m != nil {
				t.Errorf("%q: unexpected match %v", in, m)
			}
			continue
		}
		if m == nil || m[1] != want[0] || m[2] != want[1] {
			t.Errorf("%q: got %v, want pid %s host %s", in, m, want[0], want[1])
		}
	}
}
