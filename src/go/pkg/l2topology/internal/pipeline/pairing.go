// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
)

type matchedPairMetadata struct {
	id   string
	pass string
}

func canonicalAdjacencyPairID(protocol, leftDeviceID, leftPort, rightDeviceID, rightPort string) string {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	leftKey := topologyMatchCompositeKey(strings.TrimSpace(leftDeviceID), strings.TrimSpace(leftPort))
	rightKey := topologyMatchCompositeKey(strings.TrimSpace(rightDeviceID), strings.TrimSpace(rightPort))
	if protocol == "" || leftKey == "" || rightKey == "" {
		return ""
	}
	if rightKey < leftKey {
		leftKey, rightKey = rightKey, leftKey
	}
	return protocol + ":" + leftKey + "<->" + rightKey
}

func applyAdjacencyPairMetadata(adj *model.Adjacency, metadata matchedPairMetadata) {
	if adj == nil || metadata.id == "" {
		return
	}
	if adj.Labels == nil {
		adj.Labels = make(map[string]string)
	}
	adj.Labels[adjacencyLabelPairID] = metadata.id
	if metadata.pass != "" {
		adj.Labels[adjacencyLabelPairPass] = metadata.pass
	}
}

func addAdjacency(adjacencies map[string]model.Adjacency, adj model.Adjacency) bool {
	sourceID := strings.TrimSpace(adj.SourceID)
	targetID := strings.TrimSpace(adj.TargetID)
	if sourceID == "" || targetID == "" {
		return false
	}
	if sourceID == targetID {
		sourcePort := strings.TrimSpace(adj.SourcePort)
		targetPort := strings.TrimSpace(adj.TargetPort)
		if sourcePort == "" || targetPort == "" || sourcePort == targetPort {
			return false
		}
	}
	key := adjacencyKey(adj)
	if _, ok := adjacencies[key]; ok {
		return false
	}
	adjacencies[key] = adj
	return true
}

func addAttachment(attachments map[string]model.Attachment, attachment model.Attachment) bool {
	if strings.TrimSpace(attachment.DeviceID) == "" || strings.TrimSpace(attachment.EndpointID) == "" {
		return false
	}
	key := attachmentKey(attachment)
	if _, ok := attachments[key]; ok {
		return false
	}
	attachments[key] = attachment
	return true
}
