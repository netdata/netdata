// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/graph"
)

func topologyLinkInferenceValue(link topologyLink) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Inference)
}

func topologyLinkConfidenceValue(link topologyLink) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.Confidence)
}

func topologyLinkAttachmentModeValue(link topologyLink) string {
	if link.Inference == nil {
		return ""
	}
	return strings.TrimSpace(link.Inference.AttachmentMode)
}

func ensureTopologyLinkInference(link *topologyLink) *graph.LinkInference {
	if link == nil {
		return nil
	}
	if link.Inference == nil {
		link.Inference = &graph.LinkInference{}
	}
	return link.Inference
}
