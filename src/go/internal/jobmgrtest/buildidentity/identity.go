package buildidentity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	IBMManifestSchema      = "jobmgrtest-ibm-build-v1"
	ibmCommandImportPath   = "github.com/netdata/netdata/go/plugins/cmd/ibmdplugin"
	maximumManifestBytes   = 1024 * 1024
	maximumExecutableBytes = 512 * 1024 * 1024
	requiredIBMBuildTag    = "ibm_mq"
	requiredIBMCGOEnabled  = "1"
)

type Source struct {
	Revision    string `json:"revision"`
	GoTree      string `json:"go_tree"`
	GoModSHA256 string `json:"go_mod_sha256"`
	GoSumSHA256 string `json:"go_sum_sha256"`
}

type Binary struct {
	GoVersion          string `json:"go_version"`
	CommandPath        string `json:"command_path"`
	VCSRevision        string `json:"vcs_revision"`
	VCSModified        bool   `json:"vcs_modified"`
	CGOEnabled         string `json:"cgo_enabled"`
	Tags               string `json:"tags"`
	DependenciesSHA256 string `json:"dependencies_sha256"`
}

type IBMManifest struct {
	Schema           string `json:"schema"`
	Executable       string `json:"executable"`
	ExecutableSHA256 string `json:"executable_sha256"`
	Source           Source `json:"source"`
	Binary           Binary `json:"binary"`
}

func CurrentSource(ctx context.Context, goRoot string) (Source, error) {
	if ctx == nil || !filepath.IsAbs(goRoot) {
		return Source{}, errors.New(
			"jobmgr build identity: invalid Go source root",
		)
	}
	if err := requireRegular(filepath.Join(goRoot, "go.mod"), 16*1024*1024); err != nil {
		return Source{}, errors.New(
			"jobmgr build identity: Go module root is unavailable",
		)
	}
	status, err := gitOutput(
		ctx,
		goRoot,
		"status",
		"--porcelain=v1",
		"--untracked-files=all",
		"--",
		".",
	)
	if err != nil {
		return Source{}, err
	}
	if strings.TrimSpace(status) != "" {
		return Source{}, errors.New(
			"jobmgr build identity: Go source tree must be clean",
		)
	}
	revision, err := gitOutput(ctx, goRoot, "rev-parse", "HEAD")
	if err != nil {
		return Source{}, err
	}
	prefix, err := gitOutput(ctx, goRoot, "rev-parse", "--show-prefix")
	if err != nil {
		return Source{}, err
	}
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "/")
	treeRef := "HEAD"
	if prefix != "" {
		treeRef += ":" + prefix
	}
	tree, err := gitOutput(ctx, goRoot, "rev-parse", treeRef)
	if err != nil {
		return Source{}, err
	}
	goModSHA256, err := fileSHA256(
		filepath.Join(goRoot, "go.mod"),
		16*1024*1024,
	)
	if err != nil {
		return Source{}, err
	}
	goSumSHA256, err := fileSHA256(
		filepath.Join(goRoot, "go.sum"),
		64*1024*1024,
	)
	if err != nil {
		return Source{}, err
	}
	return Source{
		Revision:    strings.TrimSpace(revision),
		GoTree:      strings.TrimSpace(tree),
		GoModSHA256: goModSHA256,
		GoSumSHA256: goSumSHA256,
	}, nil
}

func GenerateIBMManifest(
	ctx context.Context,
	goRoot string,
	executable string,
	manifestPath string,
) error {
	if ctx == nil ||
		!filepath.IsAbs(executable) ||
		!filepath.IsAbs(manifestPath) {
		return errors.New("jobmgr build identity: paths must be absolute")
	}
	relative, err := filepath.Rel(
		filepath.Dir(manifestPath),
		executable,
	)
	if err != nil ||
		filepath.IsAbs(relative) ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) ||
		filepath.ToSlash(filepath.Clean(relative)) != filepath.ToSlash(relative) {
		return errors.New(
			"jobmgr build identity: IBM executable must be contained beside its manifest",
		)
	}
	source, err := CurrentSource(ctx, goRoot)
	if err != nil {
		return err
	}
	binary, err := readIBMBinary(executable, source)
	if err != nil {
		return err
	}
	executableSHA256, err := fileSHA256(
		executable,
		maximumExecutableBytes,
	)
	if err != nil {
		return err
	}
	manifest := IBMManifest{
		Schema:           IBMManifestSchema,
		Executable:       filepath.ToSlash(relative),
		ExecutableSHA256: executableSHA256,
		Source:           source,
		Binary:           binary,
	}
	file, err := os.OpenFile(
		manifestPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0o600,
	)
	if err != nil {
		return err
	}
	return errors.Join(
		json.NewEncoder(file).Encode(manifest),
		file.Close(),
	)
}

func VerifyIBMManifest(
	ctx context.Context,
	goRoot string,
	manifestPath string,
) (string, error) {
	if ctx == nil || !filepath.IsAbs(manifestPath) {
		return "", errors.New(
			"jobmgr build identity: IBM manifest path must be absolute",
		)
	}
	if err := requireRegular(manifestPath, maximumManifestBytes); err != nil {
		return "", err
	}
	file, err := os.Open(manifestPath)
	if err != nil {
		return "", err
	}
	limited := io.LimitReader(file, maximumManifestBytes+1)
	decoder := json.NewDecoder(limited)
	var manifest IBMManifest
	decodeErr := decoder.Decode(&manifest)
	var trailing any
	trailingErr := decoder.Decode(&trailing)
	closeErr := file.Close()
	if decodeErr != nil {
		return "", decodeErr
	}
	if !errors.Is(trailingErr, io.EOF) {
		if trailingErr == nil {
			trailingErr = errors.New(
				"jobmgr build identity: IBM manifest has trailing JSON",
			)
		}
		return "", errors.Join(trailingErr, closeErr)
	}
	if closeErr != nil {
		return "", closeErr
	}
	if manifest.Schema != IBMManifestSchema {
		return "", errors.New(
			"jobmgr build identity: unsupported IBM manifest",
		)
	}
	source, err := CurrentSource(ctx, goRoot)
	if err != nil {
		return "", err
	}
	if manifest.Source != source {
		return "", errors.New(
			"jobmgr build identity: IBM source identity differs",
		)
	}
	executable, err := containedManifestPath(
		filepath.Dir(manifestPath),
		manifest.Executable,
	)
	if err != nil {
		return "", err
	}
	executableSHA256, err := fileSHA256(
		executable,
		maximumExecutableBytes,
	)
	if err != nil {
		return "", err
	}
	if executableSHA256 != manifest.ExecutableSHA256 {
		return "", errors.New(
			"jobmgr build identity: IBM executable digest differs",
		)
	}
	binary, err := readIBMBinary(executable, source)
	if err != nil {
		return "", err
	}
	if manifest.Binary != binary {
		return "", errors.New(
			"jobmgr build identity: IBM binary identity differs",
		)
	}
	return executable, nil
}

func readIBMBinary(path string, source Source) (Binary, error) {
	if err := requireRegular(path, maximumExecutableBytes); err != nil {
		return Binary{}, err
	}
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return Binary{}, fmt.Errorf(
			"jobmgr build identity: read IBM build info: %w",
			err,
		)
	}
	settings := make(map[string]string, len(info.Settings))
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	modified, ok := parseBool(settings["vcs.modified"])
	if !ok ||
		info.Path != ibmCommandImportPath ||
		settings["vcs.revision"] != source.Revision ||
		modified ||
		settings["CGO_ENABLED"] != requiredIBMCGOEnabled ||
		!containsBuildTag(settings["-tags"], requiredIBMBuildTag) {
		return Binary{}, errors.New(
			"jobmgr build identity: IBM binary is not an exact clean-tree CGO ibm_mq build",
		)
	}
	return Binary{
		GoVersion:          info.GoVersion,
		CommandPath:        info.Path,
		VCSRevision:        settings["vcs.revision"],
		VCSModified:        modified,
		CGOEnabled:         settings["CGO_ENABLED"],
		Tags:               canonicalBuildTags(settings["-tags"]),
		DependenciesSHA256: dependencySHA256(info),
	}, nil
}

func containedManifestPath(directory, name string) (string, error) {
	if name == "" ||
		strings.Contains(name, "\\") ||
		filepath.IsAbs(name) ||
		filepath.Clean(name) == "." ||
		filepath.Clean(name) == ".." ||
		strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) ||
		filepath.ToSlash(filepath.Clean(name)) != name {
		return "", errors.New(
			"jobmgr build identity: unsafe IBM executable path",
		)
	}
	path := filepath.Join(directory, filepath.FromSlash(name))
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", errors.New(
			"jobmgr build identity: IBM executable is unavailable",
		)
	}
	return path, nil
}

func dependencySHA256(info *buildinfo.BuildInfo) string {
	records := make([]string, 0, len(info.Deps))
	for _, dependency := range info.Deps {
		record := dependency.Path + "\t" +
			dependency.Version + "\t" +
			dependency.Sum
		if dependency.Replace != nil {
			record += "\t" +
				dependency.Replace.Path + "\t" +
				dependency.Replace.Version + "\t" +
				dependency.Replace.Sum
		}
		records = append(records, record)
	}
	sort.Strings(records)
	digest := sha256.New()
	for _, record := range records {
		_, _ = io.WriteString(digest, record)
		_, _ = io.WriteString(digest, "\n")
	}
	return hex.EncodeToString(digest.Sum(nil))
}

func canonicalBuildTags(value string) string {
	fields := strings.FieldsFunc(
		value,
		func(r rune) bool { return r == ',' || r == ' ' },
	)
	sort.Strings(fields)
	return strings.Join(fields, ",")
}

func containsBuildTag(value, want string) bool {
	for _, tag := range strings.Split(canonicalBuildTags(value), ",") {
		if tag == want {
			return true
		}
	}
	return false
}

func parseBool(value string) (bool, bool) {
	switch value {
	case "true":
		return true, true
	case "false":
		return false, true
	default:
		return false, false
	}
}

func gitOutput(
	ctx context.Context,
	directory string,
	arguments ...string,
) (string, error) {
	command := exec.CommandContext(ctx, "git", arguments...)
	command.Dir = directory
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LANG=C",
		"LC_ALL=C",
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf(
			"jobmgr build identity: git %s: %w: %s",
			strings.Join(arguments, " "),
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return stdout.String(), nil
}

func requireRegular(path string, maximum int64) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New(
			"jobmgr build identity: artifact is not a regular file",
		)
	}
	if info.Size() > maximum {
		return errors.New(
			"jobmgr build identity: artifact exceeds size bound",
		)
	}
	return nil
}

func fileSHA256(path string, maximum int64) (string, error) {
	if err := requireRegular(path, maximum); err != nil {
		return "", err
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	written, err := io.Copy(digest, io.LimitReader(file, maximum+1))
	if err != nil {
		return "", err
	}
	if written > maximum {
		return "", errors.New(
			"jobmgr build identity: artifact exceeds size bound",
		)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
