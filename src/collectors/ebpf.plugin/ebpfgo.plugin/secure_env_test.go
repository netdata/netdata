package main

import (
	"os"
	"testing"
)

// A plain process must be treated as unprivileged so the daemon's environment is
// honoured. Running the tests as root counts as plain here: real and effective ids
// match, and a root caller is not a privilege boundary. This is what keeps a broken
// probe from silently making every plugin ignore its environment.
func TestGainedPrivilegesAtExecIsFalseForAPlainProcess(t *testing.T) {
	if os.Geteuid() != os.Getuid() || os.Getegid() != os.Getgid() {
		t.Skip("test binary is setuid/setgid")
	}

	if gainedPrivilegesAtExec() {
		t.Fatalf("gainedPrivilegesAtExec() = true for a plain process (euid %d, uid %d)",
			os.Geteuid(), os.Getuid())
	}
}

func TestSecureGetenv(t *testing.T) {
	if gainedPrivilegesAtExec() {
		t.Skip("test binary runs privileged")
	}

	const fallback = "/usr/libexec/netdata/plugins.d"

	tests := map[string]struct {
		set   bool
		value string
		want  string
	}{
		"set variable wins over the fallback": {set: true, value: "/tmp/objects", want: "/tmp/objects"},
		"empty variable yields the fallback":  {set: true, value: "", want: fallback},
		"unset variable yields the fallback":  {set: false, want: fallback},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			const key = "NETDATA_TEST_SECURE_ENV"
			if tc.set {
				t.Setenv(key, tc.value)
			} else {
				os.Unsetenv(key)
			}

			if got := secureGetenv(key, fallback); got != tc.want {
				t.Fatalf("secureGetenv(%s) = %q, want %q", key, got, tc.want)
			}
		})
	}
}
