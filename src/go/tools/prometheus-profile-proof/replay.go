// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/validation"
)

func replayValidation(
	ctx context.Context,
	input prominput.ReplayCase,
	metadataPath string,
) ([]promreplay.Result, error) {
	input.MetadataPath = metadataPath
	return promvalidation.ReplayProofCase(ctx, input)
}
