package buildidentity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Source struct {
	Revision    string `json:"revision"`
	GoTree      string `json:"go_tree"`
	GoModSHA256 string `json:"go_mod_sha256"`
	GoSumSHA256 string `json:"go_sum_sha256"`
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
