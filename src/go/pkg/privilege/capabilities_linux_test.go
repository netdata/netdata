// SPDX-License-Identifier: GPL-3.0-or-later

package privilege

import (
	"strings"
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
	capBPF  = "0000008000000000" // cap_bpf
	capPerf = "0000004000000000" // cap_perfmon
	capBoth = "000000c000000000" // cap_bpf + cap_perfmon
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
			if got, _ := gainedCapabilitiesAtExec(tc.status); got != tc.want {
				t.Fatalf("gainedCapabilitiesAtExec() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCapabilityProbeErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		data string
		want bool
	}{
		"missing permitted":                         {data: "CapEff: 0\n"},
		"invalid permitted":                         {data: "CapPrm: synthetic-invalid-value\nCapEff: 0\n"},
		"missing effective":                         {data: "CapPrm: 0\n"},
		"invalid effective":                         {data: "CapPrm: 0\nCapEff: synthetic-invalid-value\n"},
		"invalid ambient retains legacy decision":   {data: "CapPrm: 1\nCapEff: 0\nCapAmb: synthetic-invalid-value\n", want: true},
		"missing effective retains legacy decision": {data: "CapPrm: 1\n", want: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := gainedCapabilitiesAtExec([]byte(tc.data))
			if err == nil {
				t.Fatal("malformed capability state must return an error")
			}
			if strings.Contains(err.Error(), "synthetic-invalid-value") {
				t.Fatal("error exposed capability input")
			}
			if got != tc.want {
				t.Fatalf("legacy result = %v, want %v", got, tc.want)
			}
		})
	}
}
