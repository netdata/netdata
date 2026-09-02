// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"context"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
)

type S3 struct {
	mu sync.Mutex

	BucketVersioningFunc  func(context.Context, string) (s3client.BucketVersioningResult, error)
	BucketReplicationFunc func(context.Context, string) ([]s3client.ReplicationRule, error)
	PutFunc               func(context.Context, string, string, []byte, s3client.PutOptions) (s3client.PutResult, error)
	GetFunc               func(context.Context, string, string, string, int64) (s3client.GetResult, error)
	ListCurrentFunc       func(context.Context, string, string, int32) (s3client.CurrentPage, error)
	ListVersionsFunc      func(context.Context, string, string, string, string, int32) (s3client.VersionPage, error)
	DeleteFunc            func(context.Context, string, string, s3client.DeleteOptions) (s3client.DeleteResult, error)

	Calls  []Call
	Closed int
}

type Call struct {
	Operation string
	Key       string
	VersionID string
}

func (s *S3) record(call Call) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, call)
}

func (s *S3) BucketVersioning(ctx context.Context, bucket string) (s3client.BucketVersioningResult, error) {
	s.record(Call{
		Operation: "bucket_versioning",
	})
	return s.BucketVersioningFunc(ctx, bucket)
}

func (s *S3) BucketReplication(ctx context.Context, bucket string) ([]s3client.ReplicationRule, error) {
	s.record(Call{
		Operation: "bucket_replication",
	})
	return s.BucketReplicationFunc(ctx, bucket)
}

func (s *S3) Put(
	ctx context.Context,
	bucket, key string,
	payload []byte,
	opts s3client.PutOptions,
) (s3client.PutResult, error) {
	s.record(Call{
		Operation: "put",
		Key:       key,
	})
	return s.PutFunc(ctx, bucket, key, payload, opts)
}

func (s *S3) Get(
	ctx context.Context,
	bucket, key, versionID string,
	maxBytes int64,
) (s3client.GetResult, error) {
	s.record(Call{
		Operation: "get",
		Key:       key,
		VersionID: versionID,
	})
	return s.GetFunc(ctx, bucket, key, versionID, maxBytes)
}

func (s *S3) ListCurrent(
	ctx context.Context,
	bucket, prefix string,
	maxKeys int32,
) (s3client.CurrentPage, error) {
	s.record(Call{
		Operation: "list_current",
		Key:       prefix,
	})
	return s.ListCurrentFunc(ctx, bucket, prefix, maxKeys)
}

func (s *S3) ListVersions(
	ctx context.Context,
	bucket, prefix, keyMarker, versionIDMarker string,
	maxKeys int32,
) (s3client.VersionPage, error) {
	s.record(Call{
		Operation: "list_versions",
		Key:       prefix,
		VersionID: versionIDMarker,
	})
	return s.ListVersionsFunc(ctx, bucket, prefix, keyMarker, versionIDMarker, maxKeys)
}

func (s *S3) Delete(
	ctx context.Context, bucket, key string, opts s3client.DeleteOptions,
) (s3client.DeleteResult, error) {
	s.record(Call{
		Operation: "delete",
		Key:       key,
		VersionID: opts.VersionID,
	})
	return s.DeleteFunc(ctx, bucket, key, opts)
}

func (s *S3) CloseIdleConnections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Closed++
}

func (s *S3) Count(operation string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, call := range s.Calls {
		if call.Operation == operation {
			count++
		}
	}
	return count
}
