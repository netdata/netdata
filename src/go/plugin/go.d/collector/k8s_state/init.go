// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_state

import (
	"k8s.io/client-go/kubernetes"
)

func (ks *KubeState) initClient() (kubernetes.Interface, error) {
	return ks.newKubeClient()
}

func (ks *KubeState) initDiscoverer(client kubernetes.Interface) discoverer {
	return newKubeDiscovery(client, ks.Logger)
}
