// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/acquisition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfish/internal/identity"
)

func (c *Collector) collect(ctx context.Context) (result collectionResult, err error) {
	started := c.now()
	result.ObservedAt = started.UTC()
	defer func() { result.Metrics.Duration = c.now().Sub(started).Seconds() }()
	acquired, err := c.client.Acquire(ctx)
	result.AuthMethod = acquired.AuthMethod
	result.Complete = acquired.Complete
	result.Metrics.Statistics = acquired.Statistics
	if !acquired.Available {
		result.Metrics.Status = "unavailable"
		return result, err
	}
	result.Metrics.Status = "partial"
	if identity.IsIntegrityError(err) {
		result.Complete = false
		result.Diagnostics = append(acquired.Diagnostics.Values(), acquisition.BoundDiagnostic(err.Error()))
		return result, err
	}
	projected, hardwareErr := c.measurement.Project(acquired.Resources, acquired.GraphComplete, result.ObservedAt)
	result.Hardware = projected.Observations
	for _, diagnostic := range projected.Diagnostics {
		acquired.Diagnostics.Add(diagnostic)
	}
	err = errors.Join(err, hardwareErr)
	result.Complete = result.Complete && hardwareErr == nil
	result.Diagnostics = acquired.Diagnostics.Values()
	if hardwareErr != nil {
		result.Diagnostics = append(
			result.Diagnostics,
			acquisition.BoundDiagnostic("Redfish metric surface: "+hardwareErr.Error()),
		)
	}
	if result.Complete {
		result.Metrics.Status = "success"
	}
	return result, err
}
