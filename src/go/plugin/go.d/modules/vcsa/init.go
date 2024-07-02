// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/modules/vcsa/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

func (vc *VCSA) validateConfig() error {
	if vc.URL == "" {
		return errors.New("URL not set")
	}
	if vc.Username == "" || vc.Password == "" {
		return errors.New("username or password not set")
	}
	return nil
}

func (vc *VCSA) initHealthClient() (*client.Client, error) {
	httpClient, err := web.NewHTTPClient(vc.Client)
	if err != nil {
		return nil, err
	}

	return client.New(httpClient, vc.URL, vc.Username, vc.Password), nil
}
