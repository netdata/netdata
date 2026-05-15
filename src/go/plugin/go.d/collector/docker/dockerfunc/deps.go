// SPDX-License-Identifier: GPL-3.0-or-later

package dockerfunc

import (
	"context"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	typesImage "github.com/docker/docker/api/types/image"
)

// DockerClient defines the minimal Docker API surface needed by function handlers.
type DockerClient interface {
	ContainerList(context.Context, typesContainer.ListOptions) ([]types.Container, error)
	ImageList(context.Context, typesImage.ListOptions) ([]typesImage.Summary, error)
}

// Deps provides runtime dependencies needed by Docker function handlers.
type Deps interface {
	DockerClient() (DockerClient, error)
}
