// SPDX-License-Identifier: GPL-3.0-or-later

package model

import (
	"context"
)

type Discoverer interface {
	Discover(ctx context.Context, ch chan<- []TargetGroup)
}
