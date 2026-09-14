// SPDX-License-Identifier: GPL-3.0-or-later

package s3client

import (
	"context"
	"errors"
)

var (
	ErrObjectNotFound          = errors.New("S3 object not found")
	ErrReplicationConfigAbsent = errors.New("S3 replication configuration not found")
)

type VersioningStatus string

const (
	VersioningDisabled  VersioningStatus = ""
	VersioningEnabled   VersioningStatus = "Enabled"
	VersioningSuspended VersioningStatus = "Suspended"
)

type BucketVersioningResult struct {
	Status    VersioningStatus
	MFADelete bool
}

type PutResult struct {
	VersionID string
	ETag      string
}

type PutOptions struct {
	IfNoneMatch bool
}

type GetResult struct {
	Payload   []byte
	VersionID string
	ETag      string
}

type DeleteResult struct {
	VersionID    string
	DeleteMarker bool
}

type DeleteOptions struct {
	VersionID string
	IfMatch   string
}

type CurrentPage struct {
	Keys      []string
	Truncated bool
}

type VersionKind string

const (
	VersionObject       VersionKind = "object"
	VersionDeleteMarker VersionKind = "delete_marker"
)

type Version struct {
	Kind      VersionKind
	Key       string
	VersionID string
	IsLatest  bool
}

type VersionPage struct {
	Versions            []Version
	Truncated           bool
	NextKeyMarker       string
	NextVersionIDMarker string
}

type ReplicationRule struct {
	Enabled                 bool
	DestinationBucket       string
	Prefix                  string
	TagFiltered             bool
	DeleteMarkerReplication bool
	Priority                int32
}

// Client exposes only the S3 operations needed by the three probe engines.
// Provider packages use this interface so their transition tests do not need
// AWS SDK response types.
type Client interface {
	BucketVersioning(ctx context.Context, bucket string) (BucketVersioningResult, error)
	BucketReplication(ctx context.Context, bucket string) ([]ReplicationRule, error)
	Put(ctx context.Context, bucket, key string, payload []byte, opts PutOptions) (PutResult, error)
	Get(ctx context.Context, bucket, key, versionID string, maxBytes int64) (GetResult, error)
	ListCurrent(ctx context.Context, bucket, prefix string, maxKeys int32) (CurrentPage, error)
	ListVersions(
		ctx context.Context,
		bucket, prefix, keyMarker, versionIDMarker string,
		maxKeys int32,
	) (VersionPage, error)
	Delete(ctx context.Context, bucket, key string, opts DeleteOptions) (DeleteResult, error)
	CloseIdleConnections()
}
