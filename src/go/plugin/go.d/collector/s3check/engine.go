// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/aws"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/ceph"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/journal"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/probe"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/s3client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/awsauth"
)

func (c *Collector) buildEngine(ctx context.Context, agentID string) (contract.Engine, error) {
	j, err := journal.New(c.journalRoot, agentID, c.Name, c.ownershipFingerprint())
	if err != nil {
		return nil, fmt.Errorf("create ownership journal: %w", err)
	}
	source, err := c.newS3Client(ctx, c.Source.clientConfig())
	if err != nil {
		return nil, fmt.Errorf("create source S3 client: %w", err)
	}
	if !modeUsesDestination(c.Mode) {
		engine, engineErr := lifecycle.New(lifecycle.Options{
			Client:         source,
			Bucket:         c.Source.Bucket,
			Journal:        j,
			Generator:      newProbeGenerator(c.Prefix, j.OwnerID()),
			RequestTimeout: c.Source.Timeout.Duration(),
			UpdateEvery:    time.Duration(c.UpdateEvery) * time.Second,
		})
		if engineErr != nil {
			source.CloseIdleConnections()
			return nil, engineErr
		}
		return engine, nil
	}

	destination, err := c.newS3Client(ctx, c.Destination.clientConfig())
	if err != nil {
		source.CloseIdleConnections()
		return nil, fmt.Errorf("create destination S3 client: %w", err)
	}
	generator := newProbeGenerator(c.Prefix, j.OwnerID())
	var engine contract.Engine
	switch contract.Mode(c.Mode) {
	case contract.ModeCephMultisite:
		engine, err = ceph.New(ceph.Options{
			Source: source, Destination: destination,
			SourceBucket: c.Source.Bucket, DestinationBucket: c.Destination.Bucket,
			Journal: j, Generator: generator,
			SourceRequestTimeout: c.Source.Timeout.Duration(), DestinationRequestTimeout: c.Destination.Timeout.Duration(),
			WriteObjective: c.WriteObjective.Duration(), WriteTimeout: c.WriteTimeout.Duration(),
			DeleteObjective: c.DeleteObjective.Duration(), DeleteTimeout: c.DeleteTimeout.Duration(),
		})
	case contract.ModeAWSReplication:
		engine, err = aws.New(aws.Options{
			Source: source, Destination: destination,
			SourceBucket: c.Source.Bucket, DestinationBucket: c.Destination.Bucket,
			ProbePrefix: c.Prefix, Journal: j, Generator: generator,
			SourceRequestTimeout: c.Source.Timeout.Duration(), DestinationRequestTimeout: c.Destination.Timeout.Duration(),
			UpdateEvery:    time.Duration(c.UpdateEvery) * time.Second,
			WriteObjective: c.WriteObjective.Duration(), WriteTimeout: c.WriteTimeout.Duration(),
			DeleteObjective: c.DeleteObjective.Duration(), DeleteTimeout: c.DeleteTimeout.Duration(),
		})
	default:
		err = fmt.Errorf("unsupported s3check mode %q", c.Mode)
	}
	if err != nil {
		source.CloseIdleConnections()
		destination.CloseIdleConnections()
		return nil, err
	}
	return engine, nil
}

func (c S3Config) clientConfig() s3client.Config {
	return s3client.Config{
		Identity:  awsauth.NewIdentity(c.Name, c.Credentials, c.AssumeRole),
		Region:    c.Region,
		Endpoint:  c.Endpoint,
		PathStyle: boolValue(c.PathStyle),
		Timeout:   c.Timeout.Duration(),
		ProxyURL:  c.ProxyURL,
		TLS:       c.TLSConfig,
	}
}

func newProbeGenerator(prefix, ownerID string) probe.Generator {
	return probe.Generator{Prefix: prefix, OwnerID: ownerID}
}
