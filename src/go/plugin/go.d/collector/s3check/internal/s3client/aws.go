// SPDX-License-Identifier: GPL-3.0-or-later

package s3client

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
)

type Config struct {
	Identity  awsauth.Identity
	Region    string
	Endpoint  string
	PathStyle bool
	Timeout   time.Duration
	ProxyURL  string
	TLS       tlscfg.TLSConfig
}

func New(ctx context.Context, cfg Config) (Client, error) {
	httpClient, err := web.NewHTTPClient(web.ClientConfig{
		Timeout:           confopt.Duration(cfg.Timeout),
		NotFollowRedirect: true,
		ProxyURL:          cfg.ProxyURL,
		TLSConfig:         cfg.TLS,
	})
	if err != nil {
		return nil, errors.New("invalid S3 HTTP transport configuration")
	}
	awsConfig, err := cfg.Identity.NewConfig(ctx, awsauth.ConfigOptions{
		Region:     cfg.Region,
		HTTPClient: httpClient,
	})
	if err != nil {
		httpClient.CloseIdleConnections()
		return nil, fmt.Errorf("create AWS configuration: %w", err)
	}
	client := s3.NewFromConfig(awsConfig, func(options *s3.Options) { configureS3Options(options, cfg) })
	return &awsClient{
		client:     client,
		httpClient: httpClient,
	}, nil
}

func configureS3Options(options *s3.Options, cfg Config) {
	if cfg.Endpoint != "" {
		options.BaseEndpoint = aws.String(cfg.Endpoint)
	}
	options.UsePathStyle = cfg.PathStyle
}

type awsClient struct {
	client     *s3.Client
	httpClient *http.Client
}

func (c *awsClient) BucketVersioning(ctx context.Context, bucket string) (BucketVersioningResult, error) {
	out, err := c.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return BucketVersioningResult{}, err
	}
	return BucketVersioningResult{
		Status:    VersioningStatus(out.Status),
		MFADelete: out.MFADelete == types.MFADeleteStatusEnabled,
	}, nil
}

func (c *awsClient) BucketReplication(ctx context.Context, bucket string) ([]ReplicationRule, error) {
	out, err := c.client.GetBucketReplication(ctx, &s3.GetBucketReplicationInput{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		if hasErrorCode(err, "ReplicationConfigurationNotFoundError", "NoSuchReplicationConfiguration") {
			return nil, ErrReplicationConfigAbsent
		}
		return nil, err
	}
	if out.ReplicationConfiguration == nil {
		return nil, ErrReplicationConfigAbsent
	}
	rules := make([]ReplicationRule, 0, len(out.ReplicationConfiguration.Rules))
	for _, rule := range out.ReplicationConfiguration.Rules {
		rules = append(rules, convertReplicationRule(rule))
	}
	return rules, nil
}

func convertReplicationRule(rule types.ReplicationRule) ReplicationRule {
	converted := ReplicationRule{
		Enabled:  rule.Status == types.ReplicationRuleStatusEnabled,
		Priority: aws.ToInt32(rule.Priority),
	}
	if rule.Destination != nil && rule.Destination.Bucket != nil {
		converted.DestinationBucket = bucketFromARN(*rule.Destination.Bucket)
	}
	converted.DeleteMarkerReplication = rule.Filter == nil
	if rule.DeleteMarkerReplication != nil {
		converted.DeleteMarkerReplication =
			rule.DeleteMarkerReplication.Status == types.DeleteMarkerReplicationStatusEnabled
	}
	if rule.Prefix != nil {
		converted.Prefix = *rule.Prefix
	}
	if rule.Filter == nil {
		return converted
	}
	switch {
	case rule.Filter.Prefix != nil:
		converted.Prefix = *rule.Filter.Prefix
	case rule.Filter.Tag != nil:
		converted.TagFiltered = true
	case rule.Filter.And != nil:
		if rule.Filter.And.Prefix != nil {
			converted.Prefix = *rule.Filter.And.Prefix
		}
		converted.TagFiltered = len(rule.Filter.And.Tags) > 0
	}
	return converted
}

func bucketFromARN(value string) string {
	if _, bucket, ok := strings.Cut(value, ":::"); ok {
		return bucket
	}
	return value
}

func (c *awsClient) Put(
	ctx context.Context,
	bucket, key string,
	payload []byte,
	opts PutOptions,
) (PutResult, error) {
	input := &s3.PutObjectInput{
		Bucket:        aws.String(bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(payload),
		ContentLength: aws.Int64(int64(len(payload))),
	}
	if opts.IfNoneMatch {
		input.IfNoneMatch = aws.String("*")
	}
	out, err := c.client.PutObject(ctx, input)
	if err != nil {
		return PutResult{}, err
	}
	return PutResult{
		VersionID: aws.ToString(out.VersionId),
		ETag:      aws.ToString(out.ETag),
	}, nil
}

func (c *awsClient) Get(
	ctx context.Context,
	bucket, key, versionID string,
	maxBytes int64,
) (GetResult, error) {
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if versionID != "" {
		input.VersionId = aws.String(versionID)
	}
	out, err := c.client.GetObject(ctx, input)
	if err != nil {
		if isObjectNotFound(err) {
			return GetResult{}, ErrObjectNotFound
		}
		return GetResult{}, err
	}
	defer out.Body.Close() //nolint:errcheck // The bounded body is consumed or abandoned on read error.

	payload, err := io.ReadAll(io.LimitReader(out.Body, maxBytes+1))
	if err != nil {
		return GetResult{}, err
	}
	if int64(len(payload)) > maxBytes {
		return GetResult{}, fmt.Errorf("S3 object exceeds %d-byte read limit", maxBytes)
	}
	return GetResult{
		Payload:   payload,
		VersionID: aws.ToString(out.VersionId),
		ETag:      aws.ToString(out.ETag),
	}, nil
}

func (c *awsClient) ListCurrent(
	ctx context.Context,
	bucket, prefix string,
	maxKeys int32,
) (CurrentPage, error) {
	out, err := c.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	})
	if err != nil {
		return CurrentPage{}, err
	}
	page := CurrentPage{
		Keys:      make([]string, 0, len(out.Contents)),
		Truncated: aws.ToBool(out.IsTruncated),
	}
	for _, object := range out.Contents {
		if object.Key != nil {
			page.Keys = append(page.Keys, *object.Key)
		}
	}
	return page, nil
}

func (c *awsClient) ListVersions(
	ctx context.Context,
	bucket, prefix, keyMarker, versionIDMarker string,
	maxKeys int32,
) (VersionPage, error) {
	input := &s3.ListObjectVersionsInput{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(maxKeys),
	}
	if keyMarker != "" {
		input.KeyMarker = aws.String(keyMarker)
	}
	if versionIDMarker != "" {
		input.VersionIdMarker = aws.String(versionIDMarker)
	}
	out, err := c.client.ListObjectVersions(ctx, input)
	if err != nil {
		return VersionPage{}, err
	}
	page := VersionPage{
		Versions:            make([]Version, 0, len(out.Versions)+len(out.DeleteMarkers)),
		Truncated:           aws.ToBool(out.IsTruncated),
		NextKeyMarker:       aws.ToString(out.NextKeyMarker),
		NextVersionIDMarker: aws.ToString(out.NextVersionIdMarker),
	}
	for _, version := range out.Versions {
		page.Versions = append(page.Versions, Version{
			Kind:      VersionObject,
			Key:       aws.ToString(version.Key),
			VersionID: aws.ToString(version.VersionId),
			IsLatest:  aws.ToBool(version.IsLatest),
		})
	}
	for _, marker := range out.DeleteMarkers {
		page.Versions = append(page.Versions, Version{
			Kind:      VersionDeleteMarker,
			Key:       aws.ToString(marker.Key),
			VersionID: aws.ToString(marker.VersionId),
			IsLatest:  aws.ToBool(marker.IsLatest),
		})
	}
	return page, nil
}

func (c *awsClient) Delete(
	ctx context.Context, bucket, key string, opts DeleteOptions,
) (DeleteResult, error) {
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if opts.VersionID != "" {
		input.VersionId = aws.String(opts.VersionID)
	}
	if opts.IfMatch != "" {
		input.IfMatch = aws.String(opts.IfMatch)
	}
	out, err := c.client.DeleteObject(ctx, input)
	if err != nil {
		if isObjectNotFound(err) || opts.VersionID != "" && hasErrorCode(err, "NoSuchVersion") {
			return DeleteResult{}, nil
		}
		return DeleteResult{}, err
	}
	return DeleteResult{
		VersionID:    aws.ToString(out.VersionId),
		DeleteMarker: aws.ToBool(out.DeleteMarker),
	}, nil
}

func (c *awsClient) CloseIdleConnections() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}

func isObjectNotFound(err error) bool {
	if _, ok := errors.AsType[*types.NoSuchKey](err); ok {
		return true
	}
	return hasErrorCode(err, "NoSuchKey")
}

func hasErrorCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	for _, code := range codes {
		if apiErr.ErrorCode() == code {
			return true
		}
	}
	return false
}
