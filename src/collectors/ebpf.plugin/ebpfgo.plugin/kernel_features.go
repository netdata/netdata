package main

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type kernelEBPFFeatures struct {
	known        bool
	bpf          bool
	bpfSyscall   bool
	kprobesKnown bool
	kprobes      bool
}

var (
	kernelEBPFFeaturesOnce    sync.Once
	runningKernelEBPFFeatures kernelEBPFFeatures
)

var requiredKernelEBPFFeatures = []string{"CONFIG_BPF", "CONFIG_BPF_SYSCALL"}
var parsedKernelEBPFFeatures = []string{"CONFIG_BPF", "CONFIG_BPF_SYSCALL", "CONFIG_KPROBES"}

func parseKernelEBPFFeatures(config io.Reader) kernelEBPFFeatures {
	values := make(map[string]bool, len(requiredKernelEBPFFeatures))
	scanner := bufio.NewScanner(config)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "# CONFIG_") && strings.HasSuffix(line, " is not set") {
			name := strings.TrimSuffix(strings.TrimPrefix(line, "# "), " is not set")
			values[name] = false
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		if ok {
			if _, required := values[name]; required || isRequiredKernelEBPFFeature(name) {
				values[name] = value == "y"
			}
		}
	}
	if scanner.Err() != nil {
		return kernelEBPFFeatures{}
	}

	for _, name := range requiredKernelEBPFFeatures {
		if _, ok := values[name]; !ok {
			return kernelEBPFFeatures{}
		}
	}

	kprobes, kprobesKnown := values["CONFIG_KPROBES"]
	return kernelEBPFFeatures{
		known:        true,
		bpf:          values["CONFIG_BPF"],
		bpfSyscall:   values["CONFIG_BPF_SYSCALL"],
		kprobesKnown: kprobesKnown,
		kprobes:      kprobes,
	}
}

func isRequiredKernelEBPFFeature(name string) bool {
	for _, parsed := range parsedKernelEBPFFeatures {
		if name == parsed {
			return true
		}
	}
	return false
}

func kernelConfigPaths() []string {
	paths := []string{"/proc/config.gz"}
	if release, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		version := strings.TrimSpace(string(release))
		if version != "" {
			paths = append(paths,
				filepath.Join("/boot", "config-"+version),
				filepath.Join("/lib/modules", version, "build", ".config"),
			)
		}
	}
	return append(paths, "/boot/config")
}

func loadKernelEBPFFeatures() kernelEBPFFeatures {
	for _, path := range kernelConfigPaths() {
		file, err := os.Open(path)
		if err != nil {
			continue
		}

		var reader io.Reader = file
		var compressed *gzip.Reader
		if strings.HasSuffix(path, ".gz") {
			compressed, err = gzip.NewReader(file)
			if err != nil {
				file.Close()
				continue
			}
			reader = compressed
		}

		features := parseKernelEBPFFeatures(reader)
		if compressed != nil {
			compressed.Close()
		}
		file.Close()
		if features.known {
			return features
		}
	}

	return kernelEBPFFeatures{}
}

func currentKernelEBPFFeatures() kernelEBPFFeatures {
	kernelEBPFFeaturesOnce.Do(func() {
		runningKernelEBPFFeatures = loadKernelEBPFFeatures()
	})
	return runningKernelEBPFFeatures
}

func missingKernelEBPFFeatures(features kernelEBPFFeatures, requireKprobes bool) []string {
	var missing []string
	if !features.bpf {
		missing = append(missing, "CONFIG_BPF")
	}
	if !features.bpfSyscall {
		missing = append(missing, "CONFIG_BPF_SYSCALL")
	}
	if requireKprobes && features.kprobesKnown && !features.kprobes {
		missing = append(missing, "CONFIG_KPROBES")
	}
	return missing
}

func kernelEBPFFeatureError(features kernelEBPFFeatures, requireKprobes bool) error {
	if !features.known {
		return nil
	}
	missing := missingKernelEBPFFeatures(features, requireKprobes)
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("kernel lacks required eBPF build features: %s", strings.Join(missing, ", "))
}
