package buildidentity

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

func ExportCommittedGoTree(
	ctx context.Context,
	goRoot string,
	destination string,
) error {
	if ctx == nil || !filepath.IsAbs(goRoot) ||
		!filepath.IsAbs(destination) {
		return errors.New("jobmgr build identity: invalid export path")
	}
	prefix, err := gitOutput(ctx, goRoot, "rev-parse", "--show-prefix")
	if err != nil {
		return err
	}
	prefix = strings.TrimSuffix(strings.TrimSpace(prefix), "/")
	tree := "HEAD"
	if prefix != "" {
		tree += ":" + prefix
	}
	repositoryRoot, err := gitOutput(
		ctx,
		goRoot,
		"rev-parse",
		"--show-toplevel",
	)
	if err != nil {
		return err
	}
	repositoryRoot = strings.TrimSpace(repositoryRoot)
	if !filepath.IsAbs(repositoryRoot) {
		return errors.New(
			"jobmgr build identity: repository root is not absolute",
		)
	}
	if err := os.Mkdir(destination, 0o700); err != nil {
		return err
	}
	command := exec.CommandContext(
		ctx,
		"git",
		"archive",
		"--format=tar",
		tree,
	)
	command.Dir = repositoryRoot
	command.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"LANG=C",
		"LC_ALL=C",
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	archive, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	extractErr := extractCommittedTree(destination, archive)
	if extractErr != nil {
		_ = archive.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
		return extractErr
	}
	if err := command.Wait(); err != nil {
		return fmt.Errorf(
			"jobmgr build identity: git archive: %w: %s",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	if err := requireRegular(
		filepath.Join(destination, "go.mod"),
		16*1024*1024,
	); err != nil {
		return fmt.Errorf(
			"jobmgr build identity: exported Go module is unavailable: %w",
			err,
		)
	}
	return nil
}

func extractCommittedTree(destination string, source io.Reader) error {
	if !filepath.IsAbs(destination) || source == nil {
		return errors.New("jobmgr build identity: invalid archive extraction")
	}
	reader := tar.NewReader(source)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf(
				"jobmgr build identity: read source archive: %w",
				err,
			)
		}
		relative, err := safeArchivePath(header.Name)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, filepath.FromSlash(relative))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 {
				return errors.New(
					"jobmgr build identity: negative archive file size",
				)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			file, err := os.OpenFile(
				target,
				os.O_CREATE|os.O_EXCL|os.O_WRONLY,
				os.FileMode(header.Mode)&0o777,
			)
			if err != nil {
				return err
			}
			written, copyErr := io.CopyN(file, reader, header.Size)
			closeErr := file.Close()
			if copyErr != nil || written != header.Size || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
		case tar.TypeSymlink:
			if err := validateArchiveSymlink(
				relative,
				header.Linkname,
			); err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return err
			}
			if err := os.Symlink(header.Linkname, target); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"jobmgr build identity: unsupported archive entry %q",
				header.Name,
			)
		}
	}
}

func safeArchivePath(name string) (string, error) {
	trimmed := strings.TrimSuffix(name, "/")
	cleaned := path.Clean(trimmed)
	if trimmed == "" || cleaned == "." || path.IsAbs(trimmed) ||
		cleaned != trimmed || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") ||
		strings.Contains(trimmed, `\`) {
		return "", fmt.Errorf(
			"jobmgr build identity: unsafe archive path %q",
			name,
		)
	}
	return cleaned, nil
}

func validateArchiveSymlink(name, target string) error {
	cleaned := path.Clean(path.Join(path.Dir(name), target))
	if target == "" || path.IsAbs(target) || cleaned == ".." ||
		strings.HasPrefix(cleaned, "../") ||
		strings.Contains(target, `\`) {
		return fmt.Errorf(
			"jobmgr build identity: unsafe archive symlink %q",
			name,
		)
	}
	return nil
}
