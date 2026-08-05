package main

import (
	"os"
	"testing"
)

// status builds the part of /proc/self/status the capability check reads. Real
// files carry these as 16 hex digits, in this order, among many other fields.
func status(permitted, effective, ambient string) []byte {
	return []byte("Name:\tebpf-go.plugin\n" +
		"Uid:\t201\t201\t201\t201\n" +
		"CapInh:\t0000000000000000\n" +
		"CapPrm:\t" + permitted + "\n" +
		"CapEff:\t" + effective + "\n" +
		"CapBnd:\t000001ffffffffff\n" +
		"CapAmb:\t" + ambient + "\n" +
		"NoNewPrivs:\t0\n")
}

const (
	noCaps  = "0000000000000000"
	capBPF  = "0000010000000000" // cap_bpf
	capPerf = "0000008000000000" // cap_perfmon
	capBoth = "0000018000000000" // cap_bpf + cap_perfmon
)

func TestGainedCapabilitiesAtExec(t *testing.T) {
	tests := map[string]struct {
		status []byte
		want   bool
	}{
		// The case CapEff alone would miss: "setcap cap_bpf=p" leaves the
		// effective set empty, but the kernel still sets AT_SECURE, so the C side
		// refuses the environment and this side must too.
		"permitted only file capability is a gain": {
			status: status(capBPF, noCaps, noCaps),
			want:   true,
		},
		"effective file capability is a gain": {
			status: status(capBPF, capBPF, noCaps),
			want:   true,
		},
		// The caller passed cap_perfmon down as ambient, the executable added
		// cap_bpf on top: that addition is the gain.
		"permitted beyond ambient is a gain": {
			status: status(capBoth, capPerf, capPerf),
			want:   true,
		},
		"no capabilities at all is not a gain": {
			status: status(noCaps, noCaps, noCaps),
			want:   false,
		},
		// The kernel's second trigger: the file asked for effective capabilities.
		// It counts even though permitted adds nothing over ambient here, and it is
		// indistinguishable in /proc/self/status from the caller having passed the
		// same capabilities down as ambient - so both answer "gain".
		"effective capabilities with nothing gained over ambient still count": {
			status: status(capBPF, capBPF, capBPF),
			want:   true,
		},
		// Ambient passed down by the caller with the file asking for nothing
		// effective: permitted did not grow and nothing is effective, which is
		// exactly what the kernel does not treat as secure.
		"ambient capabilities alone are not a gain": {
			status: status(capBPF, noCaps, capBPF),
			want:   false,
		},
		// Old kernels have no ambient set: any permitted capability came from the
		// executable.
		"missing ambient field falls back to any permitted": {
			status: []byte("CapPrm:\t" + capBPF + "\nCapEff:\t" + noCaps + "\n"),
			want:   true,
		},
		"missing permitted field cannot decide": {
			status: []byte("CapEff:\t" + capBPF + "\n"),
			want:   false,
		},
		"unparseable permitted field cannot decide": {
			status: []byte("CapPrm:\tnot-hex\n"),
			want:   false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := gainedCapabilitiesAtExec(tc.status); got != tc.want {
				t.Fatalf("gainedCapabilitiesAtExec() = %v, want %v", got, tc.want)
			}
		})
	}
}

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
