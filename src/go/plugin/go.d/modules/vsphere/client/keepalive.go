// SPDX-License-Identifier: GPL-3.0-or-later

package client

import (
	"context"
	"net/url"
	"time"

	"github.com/vmware/govmomi"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/vim25/methods"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

const (
	keepAliveEvery = time.Second * 15
)

// TODO: survive vCenter reboot, it looks like we need to re New()
func addKeepAlive(client *govmomi.Client, userinfo *url.Userinfo) {
	f := func(rt soap.RoundTripper) error {
		_, err := methods.GetCurrentTime(context.Background(), rt)
		if err == nil {
			return nil
		}

		if !isNotAuthenticated(err) {
			return nil
		}

		_ = client.Login(context.Background(), userinfo)
		return nil
	}
	client.Client.RoundTripper = session.KeepAliveHandler(client.Client.RoundTripper, keepAliveEvery, f)
}

func isNotAuthenticated(err error) bool {
	if !soap.IsSoapFault(err) {
		return false
	}
	_, ok := soap.ToSoapFault(err).VimFault().(*types.NotAuthenticated)
	return ok
}
