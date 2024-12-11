// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"k8s.io/client-go/kubernetes"
)

func (c *Collector) initClient() (kubernetes.Interface, error) {
	return c.newKubeClient()
}

func (c *Collector) initDiscoverer(client kubernetes.Interface) discoverer {
	return newKubeDiscovery(client, c.Logger)
}
