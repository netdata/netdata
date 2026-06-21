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

func LinkOSPFLocalRouterID(link Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.LocalRouterID)
}

func LinkOSPFNeighborRouterID(link Link) string {
	if link.Detail.OSPF == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.OSPF.NeighborRouterID)
}

func LinkBGPRoutingInstance(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RoutingInstance)
}

func LinkBGPLocalIP(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIP)
}

func LinkBGPNeighborIP(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.NeighborIP)
}

func LinkBGPLocalAS(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalAS)
}

func LinkBGPRemoteAS(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.RemoteAS)
}

func LinkBGPLocalIdentifier(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.LocalIdentifier)
}

func LinkBGPPeerIdentifier(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.PeerIdentifier)
}

func LinkBGPSource(link Link) string {
	if link.Detail.BGP == nil {
		return ""
	}
	return strings.TrimSpace(link.Detail.BGP.Source)
}
