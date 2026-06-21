// SPDX-License-Identifier: GPL-3.0-or-later

package topologymodel

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func LinkInferenceValue(link Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Inference)
}

func LinkConfidenceValue(link Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Confidence)
}

func LinkAttachmentModeValue(link Link) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.AttachmentMode)
}

func EnsureLinkInference(link *Link) *graph.LinkInference {
	if link == nil {
		return nil
	}
	if link.Inference == nil {
		link.Inference = &graph.LinkInference{}
	}
	return link.Inference
}
