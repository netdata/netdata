package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	minimumKernelVersion = 4<<16 + 11<<8
	minimumRHVersion     = 7*256 + 5
)

type LoadMethod uint32

const (
	LoadLegacy LoadMethod = 1 << iota
	LoadCore
	LoadPlayDice
)

const LoadMethods = LoadLegacy | LoadCore | LoadPlayDice

type ProgramMode uint32

const (
	LoadProbe ProgramMode = iota
	LoadRetprobe
	LoadTracepoint
	LoadTrampoline
)

type RunMode uint8

const (
	RunModeEntry RunMode = iota
	RunModeDev
	RunModeReturn
)

type LoadPlan struct {
	KernelVersion uint32
	IsRHF         int
	Selector      uint32
	ObjectPath    string
	LoadMode      LoadMethod
	ProgramMode   ProgramMode
}

var kernelReleaseRe = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
var redHatReleaseRe = regexp.MustCompile(`([0-9]+)\.([0-9]+)`)

func KernelVersionFromRelease(release string) (uint32, error) {
	release = strings.TrimSpace(release)
	match := kernelReleaseRe.FindStringSubmatch(release)
	if len(match) != 4 {
		return 0, fmt.Errorf("invalid kernel release %q", release)
	}

	major, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("parse kernel major from %q: %w", release, err)
	}

	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return 0, fmt.Errorf("parse kernel minor from %q: %w", release, err)
	}

	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return 0, fmt.Errorf("parse kernel patch from %q: %w", release, err)
	}

	if patch > 255 {
		patch = 255
	}

	return uint32(major*65536 + minor*256 + patch), nil
}

func KernelVersion() (uint32, error) {
	if release, err := readFirstExistingFile("/proc/sys/kernel/osrelease"); err == nil {
		return KernelVersionFromRelease(release)
	}

	return 0, fmt.Errorf("unable to determine kernel version")
}

func RedHatReleaseFromFile(contents string) (int, error) {
	contents = strings.TrimSpace(contents)
	match := redHatReleaseRe.FindStringSubmatch(contents)
	if len(match) != 3 {
		return -1, fmt.Errorf("invalid redhat release %q", contents)
	}

	major, err := strconv.Atoi(match[1])
	if err != nil {
		return -1, fmt.Errorf("parse redhat major from %q: %w", contents, err)
	}

	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return -1, fmt.Errorf("parse redhat minor from %q: %w", contents, err)
	}

	return major*256 + minor, nil
}

func RedHatRelease() (int, error) {
	contents, err := os.ReadFile("/etc/redhat-release")
	if err != nil {
		return -1, err
	}

	return RedHatReleaseFromFile(string(contents))
}

func KernelVersionString() (string, error) {
	if content, err := readFirstExistingFile("/proc/version_signature"); err == nil {
		return content, nil
	}

	if content, err := readFirstExistingFile("/proc/version"); err == nil {
		return content, nil
	}

	return "", fmt.Errorf("unable to determine kernel version string")
}

func KernelRejected(versionString string, rejectListFiles ...string) (bool, error) {
	versionString = strings.TrimSpace(versionString)

	for _, candidate := range rejectListFiles {
		file, err := os.Open(candidate)
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			entry := strings.TrimSpace(scanner.Text())
			if entry == "" || strings.HasPrefix(entry, "#") {
				continue
			}

			if strings.HasPrefix(versionString, entry) {
				_ = file.Close()
				return true, nil
			}
		}

		if err := scanner.Err(); err != nil {
			_ = file.Close()
			return false, err
		}

		_ = file.Close()
	}

	return false, nil
}

func KernelSupported(version uint32, rhVersion int, rejected bool) bool {
	if rejected {
		return false
	}

	return version >= minimumKernelVersion || rhVersion >= minimumRHVersion
}

func CanLoadCode(version uint32, rhVersion int, rejected bool, isRoot bool, pluginName string) error {
	if !KernelSupported(version, rhVersion, rejected) {
		return fmt.Errorf("the current collector cannot run on this kernel")
	}

	if !isRoot {
		return fmt.Errorf("%s should either run as root or have special capabilities", pluginName)
	}

	return nil
}

func IsRoot() bool {
	return os.Getuid() == 0 || os.Geteuid() == 0
}

func SelectKernelName(selector uint32) string {
	kernelNames := [...]string{
		"3.10",
		"4.14",
		"4.16",
		"4.18",
		"5.4",
		"5.10",
		"5.11",
		"5.14",
		"5.15",
		"5.16",
		"6.8",
	}

	if selector >= uint32(len(kernelNames)) {
		return kernelNames[len(kernelNames)-1]
	}

	return kernelNames[selector]
}

func SelectMaxIndex(isRHF int, kver uint32) int {
	if isRHF > 0 {
		switch {
		case kver >= 331264:
			return 7
		case kver >= 328704 && kver < 328960:
			return 4
		case kver >= 264960:
			return 3
		default:
			return 0
		}
	}

	switch {
	case kver >= 395264:
		return 10
	case kver >= 331776:
		return 9
	case kver >= 331520:
		return 8
	case kver >= 330496:
		return 7
	case kver >= 330240:
		return 6
	case kver >= 328448:
		return 4
	case kver >= 265984:
		return 2
	case kver >= 264960:
		return 1
	default:
		return 0
	}
}

func SelectIndex(kernels uint32, isRHF int, kver uint32) uint32 {
	start := SelectMaxIndex(isRHF, kver)
	if isRHF == -1 {
		kernels &^= 1 << 7
	}

	for idx := start; idx > 0; idx-- {
		if kernels&(1<<uint(idx)) != 0 {
			return uint32(idx)
		}
	}

	return 0
}

func BuildObjectPath(pluginsDir string, selector uint32, name string, isReturn bool, isRHF int) string {
	prefix := 'p'
	if isReturn {
		prefix = 'r'
	}

	suffix := ""
	if isRHF != -1 {
		suffix = ".rhf"
	}

	return filepath.Join(
		pluginsDir,
		"ebpf.d",
		fmt.Sprintf("%cnetdata_ebpf_%s.%s%s.o", prefix, name, SelectKernelName(selector), suffix),
	)
}

func SelectLoadMode(hasBTF bool, load LoadMethod, kver uint32, isRH int) LoadMethod {
	if load&LoadCore != 0 || load&LoadPlayDice != 0 {
		if !hasBTF || (isRH > 0 && kver >= 328704 && kver < 328960) {
			return LoadLegacy
		}

		return LoadCore
	}

	return LoadLegacy
}

func ConvertStringToLoadMode(value string) LoadMethod {
	switch {
	case strings.EqualFold(value, "CO-RE"):
		return LoadCore
	case strings.EqualFold(value, "legacy"):
		return LoadLegacy
	default:
		return LoadPlayDice
	}
}

func ConvertCoreType(value string, mode RunMode) ProgramMode {
	switch {
	case strings.EqualFold(value, "tracepoint"):
		return LoadTracepoint
	case strings.EqualFold(value, "probe"):
		if mode == RunModeEntry {
			return LoadProbe
		}

		return LoadRetprobe
	default:
		return LoadTrampoline
	}
}

func PlanObjectPath(pluginsDir string, kernels uint32, isRHF int, kver uint32, name string, isReturn bool) LoadPlan {
	selector := SelectIndex(kernels, isRHF, kver)

	return LoadPlan{
		KernelVersion: kver,
		IsRHF:         isRHF,
		Selector:      selector,
		ObjectPath:    BuildObjectPath(pluginsDir, selector, name, isReturn, isRHF),
	}
}

func BuildLoadPlan(
	pluginsDir string,
	kernels uint32,
	isRHF int,
	kver uint32,
	name string,
	isReturn bool,
	hasBTF bool,
	load LoadMethod,
	coreAttach string,
	mode RunMode,
) LoadPlan {
	selector := SelectIndex(kernels, isRHF, kver)
	resolvedLoad := SelectLoadMode(hasBTF, load, kver, isRHF)

	return LoadPlan{
		KernelVersion: kver,
		IsRHF:         isRHF,
		Selector:      selector,
		ObjectPath:    BuildObjectPath(pluginsDir, selector, name, isReturn, isRHF),
		LoadMode:      resolvedLoad,
		ProgramMode:   ConvertCoreType(coreAttach, mode),
	}
}

func readFirstExistingFile(paths ...string) (string, error) {
	var lastErr error

	for _, path := range paths {
		contents, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}

		return strings.TrimSpace(string(contents)), nil
	}

	return "", lastErr
}
