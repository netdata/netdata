// SPDX-License-Identifier: GPL-3.0-or-later

package lighthouse

import (
	"context"
	"time"

	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

func (c *Collector) Init(ctx context.Context) error {
	return c.Job.Init(ctx, c.CheckEvery, c.UpdateEvery, c.newRequest)
}

// newRequest validates the audit options and builds the execution request.
func (c *Collector) newRequest(vnodeGUID string) (model.Request, error) {
	request := model.Request{
		Kind:      model.Lighthouse,
		VnodeGUID: vnodeGUID,
		Name:      c.Name,
		URL:       c.URL,
		Timeout:   time.Duration(c.Timeout),
		Capture:   c.SaveReport,
	}
	if err := model.ValidateRequest(request); err != nil {
		return model.Request{}, err
	}
	return request, nil
}
