// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (dh *DockerHub) validateConfig() error {
	if dh.URL == "" {
		return errors.New("url not set")
	}
	if len(dh.Repositories) == 0 {
		return errors.New("repositories not set")
	}
	return nil
}

func (dh *DockerHub) initApiClient() (*apiClient, error) {
	client, err := web.NewHTTPClient(dh.Client)
	if err != nil {
		return nil, err
	}
	return newAPIClient(client, dh.Request), nil
}
