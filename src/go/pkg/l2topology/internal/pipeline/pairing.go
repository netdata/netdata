// SPDX-License-Identifier: GPL-3.0-or-later

package pipeline

import (
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/l2topology/internal/model"
	"github.com/netdata/netdata/go/plugins/pkg/topology/worklimit"
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

func addAdjacency(limiter worklimit.Limiter, adjacencies map[string]model.Adjacency, adj model.Adjacency) (bool, error) {
	if limiter != nil {
		if err := limiter.Charge(1); err != nil {
			return false, err
		}
		bytes, err := worklimit.StringBytes(
			adj.Protocol, adj.SourceID, adj.SourcePort, adj.TargetID, adj.TargetPort,
			adj.Labels["vlan_id"], adj.Labels["vlan"],
		)
		if err != nil {
			return false, err
		}
		if err := limiter.Charge(bytes); err != nil {
			return false, err
		}
	}
	sourceID := strings.TrimSpace(adj.SourceID)
	targetID := strings.TrimSpace(adj.TargetID)
	if sourceID == "" || targetID == "" {
		return false, nil
	}
	if sourceID == targetID {
		sourcePort := strings.TrimSpace(adj.SourcePort)
		targetPort := strings.TrimSpace(adj.TargetPort)
		if sourcePort == "" || targetPort == "" || sourcePort == targetPort {
			return false, nil
		}
	}
	key := adjacencyKey(adj)
	if _, ok := adjacencies[key]; ok {
		return false, nil
	}
	adjacencies[key] = adj
	return true, nil
}

func addAttachment(limiter worklimit.Limiter, attachments map[string]model.Attachment, attachment model.Attachment) (bool, error) {
	if limiter != nil {
		if err := limiter.Charge(1); err != nil {
			return false, err
		}
		bytes, err := worklimit.StringBytes(
			attachment.DeviceID, attachment.EndpointID, attachment.Method,
			attachment.Labels["fdb_domain_id"], attachment.Labels["vlan_id"], attachment.Labels["vlan"],
		)
		if err != nil {
			return false, err
		}
		if err := limiter.Charge(bytes); err != nil {
			return false, err
		}
	}
	if strings.TrimSpace(attachment.DeviceID) == "" || strings.TrimSpace(attachment.EndpointID) == "" {
		return false, nil
	}
	key := attachmentKey(attachment)
	if _, ok := attachments[key]; ok {
		return false, nil
	}
	attachments[key] = attachment
	return true, nil
}
