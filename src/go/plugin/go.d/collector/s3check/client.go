// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type s3OperationReport struct {
	operations int
	attempts   int
}

type s3Client interface {
	GetBucketVersioning(ctx context.Context, bucket string) (string, int, error)
	PutObject(ctx context.Context, bucket, key string, payload []byte) (int, error)
	GetObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, int, error)
	ListObjects(ctx context.Context, bucket, prefix string, maxKeys int32) ([]string, bool, int, error)
	DeleteObject(ctx context.Context, bucket, key string) (int, error)
	ObjectExists(ctx context.Context, bucket, key string) (bool, s3OperationReport, error)
}

func newAWSS3Client(cfg Config) (*http.Client, s3Client, error) {
	endpoint := endpointConfig{
		Endpoint:        cfg.Endpoint,
		Region:          cfg.Region,
		Bucket:          cfg.Bucket,
		Prefix:          cfg.Prefix,
		AccessKeyID:     cfg.AccessKeyID,
		SecretAccessKey: cfg.SecretAccessKey,
		SessionToken:    cfg.SessionToken,
		PathStyle:       cfg.PathStyle,
		ClientConfig:    cfg.ClientConfig,
	}
	return newAWSS3ClientForEndpoint(endpoint, cfg.MaxRetries)
}

func newAWSS3ClientForEndpoint(cfg endpointConfig, maxRetries int) (*http.Client, s3Client, error) {
	httpClient, err := web.NewHTTPClient(cfg.ClientConfig)
	if err != nil {
		// The shared helper can echo a malformed proxy URL, including inline proxy
		// credentials. Return a bounded generic error instead.
		return nil, nil, errors.New("invalid S3 HTTP transport configuration")
	}

	retryer := newCountingBoundedRetryer(maxRetries + 1)
	awsConfig := aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, cfg.SessionToken),
		HTTPClient:  httpClient,
		Retryer: func() aws.Retryer {
			return retryer
		},
	}

	endpoint := cfg.Endpoint
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint)
		options.UsePathStyle = cfg.PathStyle
	})
	return httpClient, &awsS3Client{
		client:  client,
		retryer: retryer,
	}, nil
}

func newCountingBoundedRetryer(maxAttempts int) *countingRetryer {
	standard := retry.NewStandard(func(options *retry.StandardOptions) {
		options.MaxAttempts = maxAttempts
		options.MaxBackoff = maxRetryBackoff
	})
	return &countingRetryer{
		RetryerV2: standard,
	}
}

type countingRetryer struct {
	mu             sync.Mutex
	attemptTokens  int
	retryDecisions int

	aws.RetryerV2
}

func (r *countingRetryer) GetAttemptToken(ctx context.Context) (func(error) error, error) {
	r.mu.Lock()
	r.attemptTokens++
	r.mu.Unlock()
	return r.RetryerV2.GetAttemptToken(ctx)
}

func (r *countingRetryer) RetryDelay(attempt int, err error) (time.Duration, error) {
	r.mu.Lock()
	r.retryDecisions++
	r.mu.Unlock()
	return r.RetryerV2.RetryDelay(attempt, err)
}

func (r *countingRetryer) reset() {
	r.mu.Lock()
	r.attemptTokens = 0
	r.retryDecisions = 0
	r.mu.Unlock()
}

func (r *countingRetryer) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	// The retry middleware enters the next attempt loop before observing
	// cancellation. A retry decision therefore counts as an SDK attempt even
	// when its HTTP request is never sent.
	return max(r.attemptTokens, r.retryDecisions+1)
}

type awsS3Client struct {
	client  *s3.Client
	retryer *countingRetryer
}

func (c *awsS3Client) GetBucketVersioning(ctx context.Context, bucket string) (string, int, error) {
	c.retryer.reset()
	out, err := c.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return "", c.attemptCount(), err
	}
	return string(out.Status), c.attemptCount(), nil
}

func (c *awsS3Client) PutObject(ctx context.Context, bucket, key string, payload []byte) (int, error) {
	c.retryer.reset()
	_, err := c.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(payload),
		ContentLength: aws.Int64(int64(len(payload))),
	})
	if err != nil {
		return c.attemptCount(), err
	}
	return c.attemptCount(), nil
}

func (c *awsS3Client) GetObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, int, error) {
	c.retryer.reset()
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, c.attemptCount(), err
	}
	defer out.Body.Close() //nolint:errcheck // The response is fully read or abandoned on error.

	body, readErr := io.ReadAll(io.LimitReader(out.Body, maxBytes))
	if readErr != nil {
		return nil, c.attemptCount(), readErr
	}
	return body, c.attemptCount(), nil
}

func (c *awsS3Client) ListObjects(ctx context.Context, bucket, prefix string, maxKeys int32) ([]string, bool, int, error) {
	c.retryer.reset()
	out, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	})
	if err != nil {
		return nil, false, c.attemptCount(), err
	}

	keys := make([]string, 0, len(out.Contents))
	for _, object := range out.Contents {
		if object.Key != nil {
			keys = append(keys, *object.Key)
		}
	}
	truncated := out.IsTruncated != nil && *out.IsTruncated
	return keys, truncated, c.attemptCount(), nil
}

func (c *awsS3Client) DeleteObject(ctx context.Context, bucket, key string) (int, error) {
	c.retryer.reset()
	_, err := c.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isNoSuchKeyError(err) {
			// An already-absent object is the desired cleanup state. Some S3-compatible
			// services report NoSuchKey where Amazon S3 returns a successful idempotent delete.
			return c.attemptCount(), nil
		}
		return c.attemptCount(), err
	}
	return c.attemptCount(), nil
}

func (c *awsS3Client) ObjectExists(ctx context.Context, bucket, key string) (bool, s3OperationReport, error) {
	c.retryer.reset()
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, s3OperationReport{operations: 1, attempts: c.attemptCount()}, nil
	}
	headAttempts := c.attemptCount()
	if !isObjectAbsentError(err) {
		return false, s3OperationReport{operations: 1, attempts: headAttempts}, err
	}
	// A HEAD 404 can also mean the bucket or route disappeared. Verify the bucket
	// before treating it as proof that this object is absent.
	status, bucketAttempts, bucketErr := c.GetBucketVersioning(ctx, bucket)
	report := s3OperationReport{operations: 2, attempts: headAttempts + bucketAttempts}
	if bucketErr != nil {
		return false, report, bucketErr
	}
	if status != "" {
		return false, report, fmt.Errorf("bucket versioning status %q is unsupported during cleanup", status)
	}
	return false, report, nil
}

func (c *awsS3Client) attemptCount() int {
	if attempts := c.retryer.count(); attempts > 0 {
		return attempts
	}
	return 1
}

func isNoSuchKeyError(err error) bool {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var apiErr smithy.APIError
	return errors.As(err, &apiErr) && apiErr.ErrorCode() == "NoSuchKey"
}

func isObjectAbsentError(err error) bool {
	if isNoSuchKeyError(err) {
		return true
	}
	var notFound *types.NotFound
	return errors.As(err, &notFound)
}

func newAWSS3ClientFromDestination(cfg DestinationConfig, maxRetries int) (*http.Client, s3Client, error) {
	return newAWSS3ClientForEndpoint(destinationEndpointConfig(cfg), maxRetries)
}
