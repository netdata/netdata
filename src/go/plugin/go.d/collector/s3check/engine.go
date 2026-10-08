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

func (c *Collector) buildEngine(
	ctx context.Context,
	agentID string,
	config *selectedModeConfig,
) (contract.Engine, error) {
	j, err := journal.New(c.journalRoot, agentID, c.Name, config.ownershipFingerprint())
	if err != nil {
		return nil, fmt.Errorf("create ownership journal: %w", err)
	}
	source, err := c.newS3Client(ctx, config.Source.clientConfig())
	if err != nil {
		return nil, fmt.Errorf("create source S3 client: %w", err)
	}
	generator := probe.Generator{
		Prefix:  config.Prefix,
		OwnerID: j.OwnerID(),
	}
	if config.Mode == contract.ModeLifecycle {
		engine, engineErr := lifecycle.New(lifecycle.Options{
			Client:         source,
			Bucket:         config.Source.Bucket,
			Journal:        j,
			Generator:      generator,
			RequestTimeout: config.Source.Timeout.Duration(),
			UpdateEvery:    time.Duration(c.UpdateEvery) * time.Second,
		})
		if engineErr != nil {
			source.CloseIdleConnections()
			return nil, engineErr
		}
		return engine, nil
	}

	destination, err := c.newS3Client(ctx, config.Destination.clientConfig())
	if err != nil {
		source.CloseIdleConnections()
		return nil, fmt.Errorf("create destination S3 client: %w", err)
	}
	var engine contract.Engine
	switch config.Mode {
	case contract.ModeCephMultisite:
		engine, err = ceph.New(ceph.Options{
			Source:                    source,
			Destination:               destination,
			SourceBucket:              config.Source.Bucket,
			DestinationBucket:         config.Destination.Bucket,
			Journal:                   j,
			Generator:                 generator,
			SourceRequestTimeout:      config.Source.Timeout.Duration(),
			DestinationRequestTimeout: config.Destination.Timeout.Duration(),
			WriteObjective:            config.WriteObjective.Duration(),
			WriteTimeout:              config.WriteTimeout.Duration(),
			DeleteObjective:           config.DeleteObjective.Duration(),
			DeleteTimeout:             config.DeleteTimeout.Duration(),
		})
	case contract.ModeAWSReplication:
		engine, err = aws.New(aws.Options{
			Source:                    source,
			Destination:               destination,
			SourceBucket:              config.Source.Bucket,
			DestinationBucket:         config.Destination.Bucket,
			ProbePrefix:               config.Prefix,
			Journal:                   j,
			Generator:                 generator,
			SourceRequestTimeout:      config.Source.Timeout.Duration(),
			DestinationRequestTimeout: config.Destination.Timeout.Duration(),
			UpdateEvery:               time.Duration(c.UpdateEvery) * time.Second,
			WriteObjective:            config.WriteObjective.Duration(),
			WriteTimeout:              config.WriteTimeout.Duration(),
			DeleteObjective:           config.DeleteObjective.Duration(),
			DeleteTimeout:             config.DeleteTimeout.Duration(),
		})
	default:
		err = fmt.Errorf("unsupported s3check mode %q", config.Mode)
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
		Identity:  awsauth.NewIdentity(c.Name, c.credentialConfig(), c.AssumeRole),
		Region:    c.Region,
		Endpoint:  c.Endpoint,
		PathStyle: boolValue(c.PathStyle),
		Timeout:   c.Timeout.Duration(),
		ProxyURL:  c.ProxyURL,
		TLS:       c.TLSConfig,
	}
}
