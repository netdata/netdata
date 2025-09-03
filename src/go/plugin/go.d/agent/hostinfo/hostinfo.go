// SPDX-License-Identifier: GPL-3.0-or-later

package hostinfo

import (
	"os"
)

var (
	envKubeHost = os.Getenv("KUBERNETES_SERVICE_HOST")
	envKubePort = os.Getenv("KUBERNETES_SERVICE_PORT")
)

func IsInsideK8sCluster() bool {
	return envKubeHost != "" && envKubePort != ""
}
