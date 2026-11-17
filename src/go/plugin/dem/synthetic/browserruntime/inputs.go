// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/netdata/netdata/go/plugins/pkg/safefile"
)

// ReleaseInputs are authenticated compressed inputs retained by an installer,
// outside managed storage. They remain usable after a legacy Agent's GC.
type ReleaseInputs struct {
	Release string `json:"release"`
	Archive string `json:"archive"`
	Keyring string `json:"keyring"`
}

// SaveInputs verifies the exact pair and compressed archive before creating any
// generation or changing selection. output must be new; a failed call removes
// only the directory it created. The installer owns retention of successful inputs.
func (v VerifiedRelease) SaveInputs(ctx context.Context, expected AgentRelease, output, archive string) (_ ReleaseInputs, result error) {
	if err := v.release.ValidateForAgent(expected); err != nil {
		return ReleaseInputs{}, err
	}
	if !v.release.Manifest.MatchesHost() {
		return ReleaseInputs{}, errors.New("runtime archive does not match this architecture")
	}
	if !filepath.IsAbs(output) || len(v.envelope) == 0 || len(v.keyring) == 0 {
		return ReleaseInputs{}, errors.New("runtime inputs require an absolute new output directory and an authenticated descriptor")
	}
	if err := ctx.Err(); err != nil {
		return ReleaseInputs{}, err
	}
	var source io.ReadCloser
	var err error
	if archive != "" {
		source, err = safefile.Open(archive)
	} else {
		source, err = openDownload(ctx, v.release.Manifest.Archive.URL, downloadIdleTimeout)
	}
	if err != nil {
		return ReleaseInputs{}, err
	}
	defer source.Close()
	if err := os.Mkdir(output, 0700); err != nil {
		return ReleaseInputs{}, err
	}
	defer func() {
		if result != nil {
			result = errors.Join(result, os.RemoveAll(output))
		}
	}()
	root, err := os.OpenRoot(output)
	if err != nil {
		return ReleaseInputs{}, err
	}
	defer func() { result = errors.Join(result, root.Close()) }()
	file, err := root.OpenFile("runtime.tar.zst", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return ReleaseInputs{}, err
	}
	err = saveArchive(contextReader{ctx: ctx, source: source}, file, v.release.Manifest.Archive)
	if err = errors.Join(err, file.Close()); err != nil {
		return ReleaseInputs{}, err
	}
	if err := writeNewFile(root, "release.json", v.envelope, 0600); err != nil {
		return ReleaseInputs{}, err
	}
	if err := writeNewFile(root, "release-public.gpg", v.keyring, 0600); err != nil {
		return ReleaseInputs{}, err
	}
	if err := syncDirectory(root, "."); err != nil {
		return ReleaseInputs{}, err
	}
	parent, err := os.Open(filepath.Dir(output))
	if err != nil {
		return ReleaseInputs{}, err
	}
	if err := errors.Join(parent.Sync(), parent.Close(), ctx.Err()); err != nil {
		return ReleaseInputs{}, err
	}
	return ReleaseInputs{Release: filepath.Join(output, "release.json"), Archive: filepath.Join(output, "runtime.tar.zst"),
		Keyring: filepath.Join(output, "release-public.gpg")}, nil
}
