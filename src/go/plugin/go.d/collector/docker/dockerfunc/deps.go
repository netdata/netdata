// SPDX-License-Identifier: GPL-3.0-or-later

package dockerfunc

import (
	"context"

	docker "github.com/moby/moby/client"
)

// DockerClient defines the minimal Docker API surface needed by function handlers.
type DockerClient interface {
	ContainerList(context.Context, docker.ContainerListOptions) (docker.ContainerListResult, error)
	ImageList(context.Context, docker.ImageListOptions) (docker.ImageListResult, error)
}

// Deps provides runtime dependencies needed by Docker function handlers.
type Deps interface {
	DockerClient() (DockerClient, error)
}
