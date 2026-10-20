// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"context"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

// Deps leaves process and stream ownership with the collector.
type Deps interface {
	ExecuteFunction(context.Context, funcapi.RawMethodRequest) (*funcapi.FunctionResponse, error)
}
