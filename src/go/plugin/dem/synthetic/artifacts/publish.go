// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Finalize's caller MUST already have verified process-tree drainage. A marker
// outside the child's work folder durably records that permission. Failures before
// publication clean drained work; an attempted publication stays protected until
// both parent directories are synced or a later store owner reconciles disk.
func (s *Store) Finalize(
	ctx context.Context,
	runID string,
	captures []synthetic.Capture,
) (_ []synthetic.Artifact, retErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !runIDPattern.MatchString(runID) {
		return nil, ErrInvalidID
	}
	if !s.active[runID] {
		return nil, ErrNotFound
	}
	delete(s.active, runID)
	s.uncertain[runID] = true
	defer func() { _ = s.refresh() }()
	if err := writeSync(s.root, "drained/"+runID, []byte("1\n")); err != nil {
		return nil, err
	}
	if err := syncDir(s.root, "drained"); err != nil {
		return nil, err
	}
	delete(s.uncertain, runID)
	defer func() {
		if retErr != nil && !s.uncertain[runID] {
			if err := s.cleanupDrained(runID); err != nil {
				s.stats.CleanupErrors++
				retErr = errors.Join(retErr, err)
			}
		}
	}()
	stage := "staging/" + runID
	if err := s.root.Mkdir(stage, 0700); err != nil {
		return nil, err
	}
	if err := syncDir(s.root, "staging"); err != nil {
		return nil, err
	}
	work, err := s.root.OpenRoot("work/" + runID)
	if err != nil {
		return nil, err
	}
	defer work.Close()
	m := manifest{
		Version:   1,
		RunID:     runID,
		CreatedUS: time.Now().UnixMicro(),
		Artifacts: []synthetic.Artifact{},
	}
	ids := make(map[string]bool)
	for _, capture := range captures {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if !artifactIDPattern.MatchString(capture.ID) || ids[capture.ID] {
			return nil, fmt.Errorf("%w: duplicate or invalid ID", ErrInvalidCapture)
		}
		ids[capture.ID] = true
		if !((capture.Kind == "screenshot" && capture.MIME == "image/png") || (capture.Kind == "report" && capture.MIME == "text/html")) {
			return nil, fmt.Errorf("%w: unsupported kind or MIME", ErrInvalidCapture)
		}
		if !strings.HasPrefix(capture.Path, "output/") || path.Clean(capture.Path) != capture.Path ||
			strings.Contains(capture.Path, "\\") {
			return nil, fmt.Errorf("%w: capture must be below output/", ErrInvalidCapture)
		}
		source, err := openRegular(work, capture.Path)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidCapture, err)
		}
		info, err := source.Stat()
		if err != nil {
			return nil, errors.Join(err, source.Close())
		}
		if info.Size() > FetchMaxBytes {
			return nil, errors.Join(ErrTooLarge, source.Close())
		}
		if capture.Kind == "screenshot" {
			var header [8]byte
			_, err = io.ReadFull(source, header[:])
			if err == nil && string(header[:]) != "\x89PNG\r\n\x1a\n" {
				err = ErrInvalidCapture
			}
			if err == nil {
				_, err = source.Seek(0, io.SeekStart)
			}
			if err != nil {
				_ = source.Close()
				return nil, fmt.Errorf("%w: invalid PNG", ErrInvalidCapture)
			}
		}
		dest, err := s.root.OpenFile(stage+"/"+capture.ID, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			_ = source.Close()
			return nil, err
		}
		hash := sha256.New()
		// Published captures must be fetchable. Bound the copy too, even if the
		// source grows after Stat, rather than trusting the initial size alone.
		count, copyErr := io.Copy(
			io.MultiWriter(dest, hash),
			io.LimitReader(&contextReader{
				ctx: ctx,
				r:   source,
			}, FetchMaxBytes+1),
		)
		if count > FetchMaxBytes {
			copyErr = errors.Join(copyErr, ErrTooLarge)
		}
		closeErr := errors.Join(dest.Sync(), dest.Close(), source.Close())
		if err = errors.Join(copyErr, closeErr); err != nil {
			return nil, err
		}
		m.Artifacts = append(
			m.Artifacts,
			synthetic.Artifact{
				ID:     capture.ID,
				Kind:   capture.Kind,
				MIME:   capture.MIME,
				Bytes:  count,
				SHA256: hex.EncodeToString(hash.Sum(nil)),
			},
		)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	if err = writeSync(s.root, stage+"/manifest.json", raw); err != nil {
		return nil, err
	}
	if err = syncDir(s.root, stage); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Once publication starts, finish its durability checks even if cancellation races it.
	s.uncertain[runID] = true
	if err = s.root.Rename(stage, "runs/"+runID); err != nil {
		return nil, err
	}
	if err = errors.Join(syncDir(s.root, "runs"), syncDir(s.root, "staging")); err != nil {
		return nil, err
	}
	delete(s.uncertain, runID)
	// Work includes scripts, browser profiles and any unselected attachments.
	if err = s.cleanupDrained(runID); err != nil {
		s.stats.CleanupErrors++
	}
	return m.Artifacts, nil
}
