package main

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"
)

func TestParseKernelEBPFFeatures(t *testing.T) {
	tests := map[string]struct {
		config string
		want   kernelEBPFFeatures
	}{
		"enabled": {
			config: "CONFIG_BPF=y\nCONFIG_BPF_SYSCALL=y\nCONFIG_KPROBES=y\n",
			want:   kernelEBPFFeatures{known: true, bpf: true, bpfSyscall: true, kprobesKnown: true, kprobes: true},
		},
		"disabled kprobes": {
			config: "CONFIG_BPF=y\nCONFIG_BPF_SYSCALL=y\n# CONFIG_KPROBES is not set\n",
			want:   kernelEBPFFeatures{known: true, bpf: true, bpfSyscall: true, kprobesKnown: true},
		},
		"unknown optional kprobes": {
			config: "CONFIG_BPF=y\nCONFIG_BPF_SYSCALL=y\n",
			want:   kernelEBPFFeatures{known: true, bpf: true, bpfSyscall: true},
		},
		"required feature absent": {
			config: "CONFIG_BPF=y\n",
			want:   kernelEBPFFeatures{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := parseKernelEBPFFeatures(strings.NewReader(tc.config))
			if got != tc.want {
				t.Fatalf("parseKernelEBPFFeatures() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestKernelEBPFFeatureGate(t *testing.T) {
	features := kernelEBPFFeatures{known: true, bpf: true, bpfSyscall: true, kprobesKnown: true}
	if err := kernelEBPFFeatureError(features, false); err != nil {
		t.Fatalf("optional kprobes disabled: kernelEBPFFeatureError() = %v, want nil", err)
	}
	if err := kernelEBPFFeatureError(features, true); err == nil || !strings.Contains(err.Error(), "CONFIG_KPROBES") {
		t.Fatalf("required kprobes disabled: kernelEBPFFeatureError() = %v, want CONFIG_KPROBES error", err)
	}

	features.kprobesKnown = false
	if err := kernelEBPFFeatureError(features, true); err != nil {
		t.Fatalf("unknown kprobes config: kernelEBPFFeatureError() = %v, want nil", err)
	}

	features.bpfSyscall = false
	if err := kernelEBPFFeatureError(features, false); err == nil || !strings.Contains(err.Error(), "CONFIG_BPF_SYSCALL") {
		t.Fatalf("missing BPF syscall: kernelEBPFFeatureError() = %v, want CONFIG_BPF_SYSCALL error", err)
	}
}

func TestParseKernelEBPFFeaturesFromGzip(t *testing.T) {
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write([]byte("CONFIG_BPF=y\nCONFIG_BPF_SYSCALL=y\nCONFIG_KPROBES=y\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	r, err := gzip.NewReader(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	got := parseKernelEBPFFeatures(r)
	want := kernelEBPFFeatures{known: true, bpf: true, bpfSyscall: true, kprobesKnown: true, kprobes: true}
	if got != want {
		t.Fatalf("gzip config parse = %+v, want %+v", got, want)
	}
}
