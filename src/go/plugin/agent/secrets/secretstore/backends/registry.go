// SPDX-License-Identifier: GPL-3.0-or-later

package backends

import (
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/aws"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/azure"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/gcp"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore/backends/vault"
)

func Creators() []secretstore.Creator {
	return []secretstore.Creator{
		aws.New(),
		azure.New(),
		gcp.New(),
		vault.New(),
	}
}
