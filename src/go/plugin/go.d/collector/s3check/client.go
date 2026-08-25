// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/netdata/netdata/go/plugins/pkg/web"
)

type s3Client interface {
	GetBucketVersioning(ctx context.Context, bucket string) (string, int, error)
	PutObject(ctx context.Context, bucket, key string, payload []byte) (int, error)
	GetObject(ctx context.Context, bucket, key string, maxBytes int64) ([]byte, int, error)
	ListObjects(ctx context.Context, bucket, prefix string, maxKeys int32) ([]string, bool, int, error)
	DeleteObject(ctx context.Context, bucket, key string) (int, error)
	ObjectExists(ctx context.Context, bucket, key string) (bool, int, error)
}

func newAWSS3Client(cfg Config) (*http.Client, s3Client, error) {
	httpClient, err := web.NewHTTPClient(cfg.ClientConfig)
	if err != nil {
		return nil, nil, err
	}

	retryer := newCountingBoundedRetryer(cfg.MaxRetries + 1)
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
	return httpClient, &awsS3Client{client: client, retryer: retryer}, nil
}

func newCountingBoundedRetryer(maxAttempts int) *countingRetryer {
	standard := retry.NewStandard(func(options *retry.StandardOptions) {
		options.MaxAttempts = maxAttempts
		options.MaxBackoff = maxRetryBackoff
	})
	return &countingRetryer{RetryerV2: standard}
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
	out, err := c.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{Bucket: aws.String(bucket)})
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
		if isNotFoundError(err) {
			// An already-absent object is the desired cleanup state. Some S3-compatible
			// services report 404 where Amazon S3 returns a successful idempotent delete.
			return c.attemptCount(), nil
		}
		return c.attemptCount(), err
	}
	return c.attemptCount(), nil
}

func (c *awsS3Client) ObjectExists(ctx context.Context, bucket, key string) (bool, int, error) {
	c.retryer.reset()
	_, err := c.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err == nil {
		return true, c.attemptCount(), nil
	}
	if isNotFoundError(err) {
		return false, c.attemptCount(), nil
	}
	return false, c.attemptCount(), err
}

func (c *awsS3Client) attemptCount() int {
	if attempts := c.retryer.count(); attempts > 0 {
		return attempts
	}
	return 1
}

func isNotFoundError(err error) bool {
	var noSuchKey *types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return true
	}
	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return true
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	return false
}
