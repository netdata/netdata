// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAWSS3ClientUsesSigV4AndPathStyleRequests(t *testing.T) {
	var mu sync.Mutex
	requests := make([]capturedRequest, 0, 6)
	var putBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requests = append(requests, capturedRequest{
			method:        r.Method,
			path:          r.URL.Path,
			query:         r.URL.RawQuery,
			authorization: r.Header.Get("Authorization"),
			date:          r.Header.Get("X-Amz-Date"),
			contentSHA256: r.Header.Get("X-Amz-Content-Sha256"),
		})
		mu.Unlock()

		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "versioning"):
			writeXML(w, `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></VersioningConfiguration>`)
		case r.Method == http.MethodPut && r.URL.Path == "/probe-bucket/netdata-s3check/probe.bin":
			mu.Lock()
			putBody = body
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && r.URL.Path == "/probe-bucket/netdata-s3check/probe.bin":
			_, _ = w.Write([]byte("probe-payload"))
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			writeXML(w, fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>netdata-s3check</Name>
  <Prefix>netdata-s3check/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>100</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents><Key>netdata-s3check/probe.bin</Key></Contents>
</ListBucketResult>`))
		case r.Method == http.MethodDelete && r.URL.Path == "/probe-bucket/netdata-s3check/probe.bin":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodHead && r.URL.Path == "/probe-bucket/netdata-s3check/probe.bin":
			w.Header().Set("Content-Length", "13")
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	cfg.Bucket = "probe-bucket"
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)
	ctx := context.Background()

	versioning, attempts, err := client.GetBucketVersioning(ctx, cfg.Bucket)
	require.NoError(t, err)
	assert.Empty(t, versioning)
	assert.Equal(t, 1, attempts)

	putAttempts, err := client.PutObject(ctx, cfg.Bucket, cfg.Prefix+"probe.bin", []byte("probe-payload"))
	require.NoError(t, err)
	assert.Equal(t, 1, putAttempts)
	mu.Lock()
	capturedPutBody := putBody
	mu.Unlock()
	assert.Equal(t, []byte("probe-payload"), capturedPutBody)

	payload, getAttempts, err := client.GetObject(ctx, cfg.Bucket, cfg.Prefix+"probe.bin", 14)
	require.NoError(t, err)
	assert.Equal(t, []byte("probe-payload"), payload)
	assert.Equal(t, 1, getAttempts)

	keys, truncated, listAttempts, err := client.ListObjects(ctx, cfg.Bucket, cfg.Prefix, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{cfg.Prefix + "probe.bin"}, keys)
	assert.False(t, truncated)
	assert.Equal(t, 1, listAttempts)

	deleteAttempts, err := client.DeleteObject(ctx, cfg.Bucket, cfg.Prefix+"probe.bin")
	require.NoError(t, err)
	assert.Equal(t, 1, deleteAttempts)

	exists, headReport, err := client.ObjectExists(ctx, cfg.Bucket, cfg.Prefix+"probe.bin")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, s3OperationReport{operations: 1, attempts: 1}, headReport)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, requests, 6)
	for _, request := range requests {
		assert.Contains(t, request.authorization, "AWS4-HMAC-SHA256 Credential=test-access-key-id/")
		assert.NotContains(t, request.authorization, "test-secret-access-key")
		assert.Regexp(t, `^\d{8}T\d{6}Z$`, request.date)
		assert.NotEmpty(t, request.contentSHA256)
		assert.True(t, strings.HasPrefix(request.path, "/probe-bucket"), request.path)
	}
	assert.Equal(t, []string{"GET", "PUT", "GET", "GET", "DELETE", "HEAD"}, requestMethods(requests))
}

func TestAWSS3ClientAccountsSDKRetryAttempts(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/probe-bucket/netdata-s3check/probe.bin" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if atomic.AddInt32(&attempts, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	cfg.Bucket = "probe-bucket"
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)

	requestAttempts, err := client.PutObject(context.Background(), cfg.Bucket, cfg.Prefix+"probe.bin", []byte("payload"))
	require.NoError(t, err)
	assert.Equal(t, 2, requestAttempts)
	assert.Equal(t, int32(2), atomic.LoadInt32(&attempts))
}

func TestAWSS3ClientBoundsRetryBackoff(t *testing.T) {
	cfg := validTestConfig()
	cfg.Endpoint = "http://127.0.0.1:9000"
	retryer := newCountingBoundedRetryer(cfg.MaxRetries + 1)
	require.Equal(t, cfg.MaxRetries+1, retryer.MaxAttempts())
	for attempt := 1; attempt <= cfg.MaxRetries; attempt++ {
		delay, err := retryer.RetryDelay(attempt, nil)
		require.NoError(t, err)
		assert.LessOrEqual(t, delay, maxRetryBackoff)
		assert.GreaterOrEqual(t, delay, time.Duration(0))
	}
}

func TestAWSS3ClientAccountsAttemptCancelledDuringRetryBackoff(t *testing.T) {
	t.Setenv("AWS_NEW_RETRIES_2026", "true")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Amz-Retry-After", "60000")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	cfg.Bucket = "probe-bucket"
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	attempts, err := client.PutObject(ctx, cfg.Bucket, cfg.Prefix+"probe.bin", []byte("payload"))
	require.Error(t, err)
	assert.Equal(t, 2, attempts)
}

func TestAWSS3ClientTreatsHead404AsMissingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "versioning") {
			writeXML(w, `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"></VersioningConfiguration>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)

	exists, report, err := client.ObjectExists(context.Background(), cfg.Bucket, cfg.Prefix+"probe.bin")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Equal(t, s3OperationReport{operations: 2, attempts: 2}, report)
}

type capturedRequest struct {
	method        string
	path          string
	query         string
	authorization string
	date          string
	contentSHA256 string
}

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(body))
}

func requestMethods(requests []capturedRequest) []string {
	out := make([]string, len(requests))
	for i, request := range requests {
		out[i] = request.method
	}
	return out
}

func TestAWSS3ClientUsesDestinationCredentialsAndPathStyle(t *testing.T) {
	var mu sync.Mutex
	var request capturedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = body
		mu.Lock()
		request = capturedRequest{
			method:        r.Method,
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			date:          r.Header.Get("X-Amz-Date"),
			contentSHA256: r.Header.Get("X-Amz-Content-Sha256"),
		}
		mu.Unlock()
		_, _ = w.Write([]byte("destination-payload"))
	}))
	defer server.Close()

	destination := validMultisiteTestConfig().Destination
	destination.Endpoint = server.URL
	_, client, err := newAWSS3ClientFromDestination(*destination, defaultMaxRetries)
	require.NoError(t, err)

	payload, attempts, err := client.GetObject(context.Background(), destination.Bucket, destination.Prefix+"probe.bin", 20)
	require.NoError(t, err)
	assert.Equal(t, []byte("destination-payload"), payload)
	assert.Equal(t, 1, attempts)
	mu.Lock()
	capturedRequest := request
	mu.Unlock()
	assert.Equal(t, http.MethodGet, capturedRequest.method)
	assert.True(t, strings.HasPrefix(capturedRequest.path, "/"+destination.Bucket+"/"), capturedRequest.path)
	assert.Contains(t, capturedRequest.authorization, "AWS4-HMAC-SHA256 Credential=test-destination-access-key-id/")
	assert.NotContains(t, capturedRequest.authorization, "test-destination-secret-access-key")
}

func TestAWSS3ClientTreatsPlain404AsRequestFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)

	_, _, err = client.GetObject(context.Background(), cfg.Bucket, cfg.Prefix+"probe.bin", 2)
	require.Error(t, err)
	assert.False(t, isNoSuchKeyError(err))
}

func TestAWSS3ClientRejectsVersionedBucketDuringAbsenceProof(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.RawQuery, "versioning") {
			writeXML(w, `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cfg := validTestConfig()
	cfg.Endpoint = server.URL
	_, client, err := newAWSS3Client(cfg)
	require.NoError(t, err)

	exists, report, err := client.ObjectExists(context.Background(), cfg.Bucket, cfg.Prefix+"probe.bin")
	require.Error(t, err)
	assert.False(t, exists)
	assert.Equal(t, s3OperationReport{operations: 2, attempts: 2}, report)
	assert.Contains(t, err.Error(), "versioning status")
}
