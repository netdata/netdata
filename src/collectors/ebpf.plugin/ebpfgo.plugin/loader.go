package main

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	minimumKernelVersion       = 4<<16 + 11<<8
	minimumKernelVersionBuffer = 5<<16 + 10<<8
	minimumKernelVersionArena  = 6<<16 + 12<<8
	maximumKernelVersionArena  = 6<<16 + 13<<8
	minimumKernelVersionSeven  = 7 << 16
	minimumRHVersion           = 7*256 + 5
)

var errNotRedHatFamily = errors.New("host is not a recognized Red Hat-family distribution")

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
	Flavor        ObjectFlavor
	// IsReturn selects the object-file prefix: 'r' for the return-probe family,
	// 'p' otherwise.  It is carried on the plan (not just passed to the path
	// builder) so buildFallbackPlans can keep the whole fallback chain in the
	// same family as the primary plan.
	IsReturn    bool
	ObjectPath  string
	LoadMode    LoadMethod
	ProgramMode ProgramMode
}

type LoadPlanRequest struct {
	PluginsDir       string
	Kernels          uint32
	IsRHF            int
	KernelVersion    uint32
	Name             string
	IsReturn         bool
	HasResizableMaps bool
	IsDebian         bool
	HasBTF           bool
	Load             LoadMethod
	CoreAttach       string
	Mode             RunMode
}

type ObjectFlavor string

const (
	ObjectFlavorBase   ObjectFlavor = ""
	ObjectFlavorBuffer ObjectFlavor = "buffer"
	ObjectFlavorArena  ObjectFlavor = "arena"
)

var kernelReleaseRe = regexp.MustCompile(`^([0-9]+)\.([0-9]+)\.([0-9]+)`)
var redHatReleaseRe = regexp.MustCompile(`(?i)^\s*(red hat|centos|almalinux|rocky linux|cloudlinux).*?release[[:space:]]+([0-9]+)(\.([0-9]+))?`)

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
	if len(match) != 5 {
		return -1, fmt.Errorf("invalid redhat release %q", contents)
	}

	major, err := strconv.Atoi(match[2])
	if err != nil {
		return -1, fmt.Errorf("parse redhat major from %q: %w", contents, err)
	}

	minor := 0
	if match[4] != "" {
		minor, err = strconv.Atoi(match[4])
		if err != nil {
			return -1, fmt.Errorf("parse redhat minor from %q: %w", contents, err)
		}
	}

	return major*256 + minor, nil
}

func RedHatRelease() (int, error) {
	contents, err := os.ReadFile("/etc/redhat-release")
	if err == nil {
		if version, parseErr := RedHatReleaseFromFile(string(contents)); parseErr == nil {
			return version, nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		redHatErr := fmt.Errorf("unable to read /etc/redhat-release: %w", err)
		contents, osReleaseErr := os.ReadFile("/etc/os-release")
		return redHatReleaseFromUnreadableFile(redHatErr, string(contents), osReleaseErr)
	}

	contents, err = os.ReadFile("/etc/os-release")
	if err != nil {
		return -1, fmt.Errorf("unable to read Red Hat release metadata: %w", err)
	}
	if version := RedHatReleaseFromOSRelease(string(contents)); version >= 0 {
		return version, nil
	}
	return -1, errNotRedHatFamily
}

func redHatReleaseFromUnreadableFile(redHatErr error, osRelease string, osReleaseErr error) (int, error) {
	if osReleaseErr != nil {
		// Keep the primary read error discoverable; a secondary ENOENT would hide it from errors.Is callers.
		return -1, fmt.Errorf("%w (also unable to read /etc/os-release: %v)", redHatErr, osReleaseErr)
	}
	if version := RedHatReleaseFromOSRelease(osRelease); version >= 0 {
		return version, nil
	}
	return -1, redHatErr
}

func RedHatReleaseFromOSRelease(contents string) int {
	var major, minor int
	isRHEL := false
	for _, line := range strings.Split(contents, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, "\"'")
		switch key {
		case "ID":
			switch strings.ToLower(value) {
			case "rhel", "centos", "almalinux", "rocky", "cloudlinux":
				isRHEL = true
			}
		case "VERSION_ID":
			parts := strings.Split(value, ".")
			parsedMajor, err := strconv.Atoi(parts[0])
			if err != nil || parsedMajor <= 0 {
				return -1
			}
			major = parsedMajor
			if len(parts) >= 2 {
				parsedMinor, err := strconv.Atoi(parts[1])
				if err != nil || parsedMinor < 0 || parsedMinor > 255 {
					return -1
				}
				minor = parsedMinor
				for _, suffix := range parts[2:] {
					if _, err := strconv.Atoi(suffix); err != nil {
						return -1
					}
				}
			}
		}
	}
	if isRHEL && major > 0 && major <= (int(^uint(0)>>1)-255)/256 {
		return major*256 + minor
	}
	return -1
}

// KernelVersionString returns a version string suitable for reject-list prefix
// matching.  It tries /proc/version_signature first (Ubuntu-only; returns
// "Ubuntu 4.18.0-xx.xx-generic 4.18.20"), then falls back to
// /proc/sys/kernel/osrelease (returns "4.18.0-xxx-generic" on other distros).
// Using version_signature as the primary source preserves Ubuntu-specific
// reject-list entries (e.g. "Ubuntu 4.18.0") without matching the same kernel
// version on non-Ubuntu distributions like RHEL 8 ("4.18.0-305.el8.x86_64").
func KernelVersionString() (string, error) {
	if release, err := readFirstExistingFile(
		"/proc/version_signature",
		"/proc/sys/kernel/osrelease",
	); err == nil {
		return release, nil
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

func DebianFlavorFromOSRelease(contents string) bool {
	scanner := bufio.NewScanner(strings.NewReader(contents))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key != "ID" {
			continue
		}

		value = strings.Trim(value, `"'`)
		if value == "debian" {
			return true
		}
	}

	return false
}

func IsDebianFlavor() bool {
	contents, err := readFirstExistingFile("/etc/os-release", "/usr/lib/os-release")
	if err != nil {
		return false
	}

	return DebianFlavorFromOSRelease(contents)
}

func KernelSupported(version uint32, rhVersion int, rejected bool) bool {
	if rejected {
		return false
	}

	return version >= minimumKernelVersion || rhVersion >= minimumRHVersion
}

func CanLoadCode(version uint32, rhVersion int, rejected bool, isRoot bool, requireKprobes bool, pluginName string) error {
	if rejected {
		return fmt.Errorf("the current collector cannot run on this rejected kernel")
	}
	if err := kernelEBPFFeatureError(currentKernelEBPFFeatures(), requireKprobes); err != nil {
		return err
	}
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
		"6.12",
		"7.0",
	}

	if selector >= uint32(len(kernelNames)) {
		return kernelNames[len(kernelNames)-1]
	}

	return kernelNames[selector]
}

func isRHEL10Kernel612Stream(isRHF int, kver uint32) bool {
	return isRHF >= 10*256 && isRHF < 11*256 &&
		kver >= minimumKernelVersionArena && kver < maximumKernelVersionArena
}

func SelectMaxIndex(isRHF int, kver uint32) int {
	if isRHF > 0 {
		switch {
		case isRHEL10Kernel612Stream(isRHF, kver):
			return 11
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
	case kver >= minimumKernelVersionSeven:
		return 12
	case kver >= minimumKernelVersionArena:
		return 11
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

func effectiveMaxBaseSelector(maxBaseSelector, isRHF int, kver uint32) int {
	if isRHEL10Kernel612Stream(isRHF, kver) {
		return 11
	}
	return maxBaseSelector
}

func BuildObjectPath(pluginsDir string, selector uint32, name string, isReturn bool, isRHF int) string {
	return BuildObjectPathWithFlavor(pluginsDir, selector, name, isReturn, isRHF, ObjectFlavorBase)
}

func SelectObjectFlavor(kver uint32, hasResizableMaps bool, isDebian bool) ObjectFlavor {
	if !hasResizableMaps {
		return ObjectFlavorBase
	}

	if kver >= minimumKernelVersionArena && !isDebian {
		return ObjectFlavorArena
	}

	if kver >= minimumKernelVersionBuffer {
		return ObjectFlavorBuffer
	}

	return ObjectFlavorBase
}

// selectConfiguredObjectFlavor applies the user-requested flavor string from
// config with a kernel-version gate.  Both cachestat and socket use the same
// policy, so a single function prevents the two callers from diverging.
func selectConfiguredObjectFlavor(requested string, kver uint32, isDebian bool) ObjectFlavor {
	switch strings.ToLower(strings.TrimSpace(requested)) {
	case "", "buffer":
		if kver >= minimumKernelVersionBuffer {
			return ObjectFlavorBuffer
		}
	case "arena":
		if kver >= minimumKernelVersionArena && !isDebian {
			return ObjectFlavorArena
		}
	}
	return ObjectFlavorBase
}

func BuildObjectPathWithFlavor(
	pluginsDir string,
	selector uint32,
	name string,
	isReturn bool,
	isRHF int,
	flavor ObjectFlavor,
) string {
	prefix := 'p'
	if isReturn {
		prefix = 'r'
	}

	objectName := name
	if flavor != ObjectFlavorBase {
		objectName = fmt.Sprintf("%s_%s", name, flavor)
	}

	suffix := ""
	if isRHF != -1 {
		suffix = ".rhf"
	}

	return filepath.Join(
		pluginsDir,
		"ebpf.d",
		fmt.Sprintf("%cnetdata_ebpf_%s.%s%s.o", prefix, objectName, SelectKernelName(selector), suffix),
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
	return BuildLoadPlan(LoadPlanRequest{
		PluginsDir:    pluginsDir,
		Kernels:       kernels,
		IsRHF:         isRHF,
		KernelVersion: kver,
		Name:          name,
		IsReturn:      isReturn,
		Load:          LoadLegacy,
		Mode:          RunModeEntry,
	})
}

func BuildLoadPlan(req LoadPlanRequest) LoadPlan {
	selector := SelectIndex(req.Kernels, req.IsRHF, req.KernelVersion)
	flavor := SelectObjectFlavor(req.KernelVersion, req.HasResizableMaps, req.IsDebian)
	resolvedLoad := SelectLoadMode(req.HasBTF, req.Load, req.KernelVersion, req.IsRHF)

	return LoadPlan{
		KernelVersion: req.KernelVersion,
		IsRHF:         req.IsRHF,
		Selector:      selector,
		Flavor:        flavor,
		// Carry the requested family on the plan, not just into the path:
		// buildFallbackPlans reads primary.IsReturn to keep the whole fallback
		// chain in the same 'p'/'r' family as the primary.
		IsReturn:    req.IsReturn,
		ObjectPath:  BuildObjectPathWithFlavor(req.PluginsDir, selector, req.Name, req.IsReturn, req.IsRHF, flavor),
		LoadMode:    resolvedLoad,
		ProgramMode: ConvertCoreType(req.CoreAttach, req.Mode),
	}
}

// resolveKernelAndRH detects the running kernel version and the RedHat-family
// release number.  isRHF is -1 in both "not RHEL" and "detection failed" cases.
// That intentional ambiguity is safe: failing to read /etc/redhat-release is
// almost always ENOENT (not RHEL), and on the rare EPERM case the caller will
// attempt to load the non-.rhf objects which will fail fast; the error is
// logged so the operator can investigate.
// Every resolve*LegacyConfig function needs this pair, so it lives here.
func resolveKernelAndRH() (kver uint32, isRHF int, err error) {
	kver, err = KernelVersion()
	if err != nil {
		return 0, -1, err
	}
	isRHF = -1
	if rhf, rherr := RedHatRelease(); rherr == nil {
		isRHF = rhf
	} else if shouldWarnRedHatDetectionError(rherr) {
		fmt.Fprintf(os.Stderr,
			"ebpf-go.plugin: warning: unable to determine Red Hat-family release metadata (%v); treating host as non-RHEL\n",
			rherr)
	}
	return kver, isRHF, nil
}

func shouldWarnRedHatDetectionError(err error) bool {
	return err != nil && !errors.Is(err, errNotRedHatFamily) && !errors.Is(err, fs.ErrNotExist)
}

// kprobePlanRequest carries the inputs shared by kprobe/trampoline-based
// collectors.  It exists solely to keep buildKprobeLegacyPlan within the
// parameter-count limit while preserving all field names at call sites.
type kprobePlanRequest struct {
	PluginsDir      string
	Kernels         uint32
	IsRHF           int
	KernelVersion   uint32
	IsDebian        bool
	HasBTF          bool
	ObjectFlavor    string
	Name            string
	MaxBaseSelector int  // highest selector index for which a base-flavor object exists
	IsReturn        bool // load the 'r'-prefixed (return-probe) object family
}

// buildKprobeLegacyPlan constructs the LoadPlan for kprobe/trampoline-based
// collectors (cachestat, socket).  DNS uses a different load mode and is not
// covered by this helper.
func buildKprobeLegacyPlan(req kprobePlanRequest) LoadPlan {
	flavor := selectConfiguredObjectFlavor(req.ObjectFlavor, req.KernelVersion, req.IsDebian)
	loadMode := SelectLoadMode(req.HasBTF, LoadCore, req.KernelVersion, req.IsRHF)
	selector := SelectIndex(req.Kernels, req.IsRHF, req.KernelVersion)
	// Base-flavor objects are not built for every kernel the buffer/arena objects
	// cover.  Cap the selector so we never construct a path that does not exist.
	baseMax := effectiveMaxBaseSelector(req.MaxBaseSelector, req.IsRHF, req.KernelVersion)
	if flavor == ObjectFlavorBase && baseMax > 0 && int(selector) > baseMax {
		selector = uint32(baseMax)
	}
	return LoadPlan{
		KernelVersion: req.KernelVersion,
		IsRHF:         req.IsRHF,
		Selector:      selector,
		Flavor:        flavor,
		IsReturn:      req.IsReturn,
		ObjectPath:    BuildObjectPathWithFlavor(req.PluginsDir, selector, req.Name, req.IsReturn, req.IsRHF, flavor),
		LoadMode:      loadMode,
		ProgramMode:   LoadTrampoline,
	}
}

// buildFallbackPlans returns plans in preference order: primary first, then
// progressively less demanding flavors (arena -> buffer -> base).  RHF plans
// also try the generic filename for each flavor because some object families
// use the RHF kernel selector without shipping .rhf-suffixed object files.
// maxBaseSelector is the highest selector index for which base-flavor objects
// exist for this module; pass 0 to disable the cap.
func buildFallbackPlans(primary LoadPlan, pluginsDir string, isRHF int, name string, maxBaseSelector int) []LoadPlan {
	maxBaseSelector = effectiveMaxBaseSelector(maxBaseSelector, isRHF, primary.KernelVersion)
	plans := make([]LoadPlan, 0, 6)
	addPlan := func(plan LoadPlan) {
		plans = append(plans, plan)
		if isRHF == -1 || isRHEL10Kernel612Stream(isRHF, primary.KernelVersion) {
			return
		}

		generic := plan
		generic.IsRHF = -1
		generic.ObjectPath = BuildObjectPathWithFlavor(pluginsDir, plan.Selector, name, plan.IsReturn, -1, plan.Flavor)
		plans = append(plans, generic)
	}

	addPlan(primary)

	if primary.Flavor == ObjectFlavorArena {
		fb := primary
		fb.Flavor = ObjectFlavorBuffer
		fb.ObjectPath = BuildObjectPathWithFlavor(pluginsDir, primary.Selector, name, primary.IsReturn, isRHF, ObjectFlavorBuffer)
		addPlan(fb)
	}

	if primary.Flavor != ObjectFlavorBase {
		fb := primary
		fb.Flavor = ObjectFlavorBase
		fbSelector := primary.Selector
		if maxBaseSelector > 0 && int(fbSelector) > maxBaseSelector {
			fbSelector = uint32(maxBaseSelector)
		}
		fb.Selector = fbSelector
		fb.ObjectPath = BuildObjectPathWithFlavor(pluginsDir, fbSelector, name, primary.IsReturn, isRHF, ObjectFlavorBase)
		addPlan(fb)
	}

	return plans
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
