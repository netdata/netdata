// SPDX-License-Identifier: GPL-3.0-or-later

package mssqlfunc

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// mssqlXEventReadTarget identifies an Extended Events session target.
type mssqlXEventReadTarget struct {
	// filePath is the event_file read pattern. Empty when reading the ring buffer.
	filePath string
	// filePrefix is the exact generated-file prefix that queryMSSQLXEventFileEvents uses to
	// exclude other sessions whose configured filename shares this one's wildcard match.
	filePrefix string
}

// fileQueryArgs binds the parameters of queryMSSQLXEventFileEvents for one event type.
func (t mssqlXEventReadTarget) fileQueryArgs(eventName string) []any {
	return []any{
		sql.Named("filePath", t.filePath),
		sql.Named("filePrefix", t.filePrefix),
		sql.Named("eventName", eventName),
	}
}

// resolveMSSQLXEventReadTarget locates the Extended Events target for a session, reporting
// false when the session or the requested target does not exist.
//
// For the event_file target the on-disk name is read from the catalog rather than derived
// from the session name: the filename is operator-chosen and the two frequently differ.
// The ring_buffer target only exists while the session is running, so that path still
// goes through the runtime DMVs.
func (r *router) resolveMSSQLXEventReadTarget(
	ctx context.Context,
	sessionName string,
	useRingBuffer bool,
) (mssqlXEventReadTarget, bool, error) {
	if !useRingBuffer {
		query := queryMSSQLXEventSessionEventFilePath
		if r.deps.ServerInfo().AzureSQLDatabase {
			query = queryMSSQLXEventDatabaseSessionEventFilePath
		}
		var configured sql.NullString
		err := r.deps.DB().QueryRowContext(ctx, query, sql.Named("sessionName", sessionName)).Scan(&configured)
		if errors.Is(err, sql.ErrNoRows) {
			return mssqlXEventReadTarget{}, false, nil
		}
		if err != nil {
			return mssqlXEventReadTarget{}, false, err
		}
		target := eventFileReadTarget(configured.String)
		if target.filePath == "" {
			return mssqlXEventReadTarget{}, false, nil
		}
		return target, true, nil
	}

	available, err := r.mssqlRingBufferAvailable(ctx, sessionName)
	return mssqlXEventReadTarget{}, available, err
}

// eventFileReadTarget turns the configured event_file filename into the read path and
// exact generated-file prefix. Local files use a wildcard; Azure Storage uses the
// wildcard-free blob prefix required by sys.fn_xe_file_target_read_file.
func eventFileReadTarget(configured string) mssqlXEventReadTarget {
	path := strings.TrimSpace(configured)
	if path == "" {
		return mssqlXEventReadTarget{}
	}
	if strings.HasSuffix(strings.ToLower(path), ".xel") {
		path = path[:len(path)-len(".xel")]
	}

	prefix := path + "_0_"
	base := strings.ReplaceAll(path, `\`, "/")
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		base = base[idx+1:]
	}
	filePrefix := base + "_0_"
	lower := strings.ToLower(path)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return mssqlXEventReadTarget{
			filePath:   prefix,
			filePrefix: filePrefix,
		}
	}
	return mssqlXEventReadTarget{
		filePath:   prefix + "*.xel",
		filePrefix: filePrefix,
	}
}

// The target query joins live sessions, so it also excludes stopped sessions.
func (r *router) mssqlRingBufferAvailable(ctx context.Context, sessionName string) (bool, error) {
	query := queryMSSQLXEventSessionHasRingBuffer
	if r.deps.ServerInfo().AzureSQLDatabase {
		query = queryMSSQLXEventDatabaseSessionHasRingBuffer
	}
	var count int
	err := r.deps.DB().QueryRowContext(ctx, query, sql.Named("sessionName", sessionName)).Scan(&count)
	return count > 0, err
}
