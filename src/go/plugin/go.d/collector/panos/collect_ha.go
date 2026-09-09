// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"fmt"
	"strings"
)

type haResult struct {
	Enabled string  `xml:"enabled"`
	Group   haGroup `xml:"group"`
}

type haGroup struct {
	Mode        string `xml:"mode"`
	RunningSync string `xml:"running-sync"`
	LocalInfo   haInfo `xml:"local-info"`
	PeerInfo    haInfo `xml:"peer-info"`
}

type haInfo struct {
	State      string `xml:"state"`
	ConnStatus string `xml:"conn-status"`
	StateSync  string `xml:"state-sync"`
	ConnHA1    haConn `xml:"conn-ha1"`
	ConnHA1B   haConn `xml:"conn-ha1-backup"`
	ConnHA2    haConn `xml:"conn-ha2"`
	ConnHA2B   haConn `xml:"conn-ha2-backup"`
}

type haConn struct {
	Status string `xml:"conn-status"`
}

func (c *Collector) collectHAMetrics(ctx context.Context) (bool, error) {
	body, err := c.apiClient.op(ctx, haStateCommand)
	if err != nil {
		return false, fmt.Errorf("ha metricset: %s API call: %w", panosCommandName(haStateCommand), err)
	}

	ha, err := parseHAState(body)
	if err != nil {
		return false, fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}
	if firstNonEmpty(ha.Enabled, ha.Group.LocalInfo.State, ha.Group.PeerInfo.State) == "" {
		return false, fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), missingPANOSResultError{expected: "<enabled> or <group>"})
	}

	enabled, err := c.haEnabledStatus(ha)
	if err != nil {
		return false, fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}
	if !enabled && firstNonEmpty(ha.Group.LocalInfo.State, ha.Group.PeerInfo.State, ha.Group.RunningSync) == "" {
		observeStateSet(c.metrics.ha.status, "disabled")
		return true, nil
	}

	localState := normalizeHAState(ha.Group.LocalInfo.State)
	peerState := normalizeHAState(ha.Group.PeerInfo.State)
	stateSync := firstNonEmpty(ha.Group.RunningSync, ha.Group.LocalInfo.StateSync)

	observeStateSet(c.metrics.ha.status, boolState(enabled, "enabled", "disabled"))
	observeStateSet(c.metrics.ha.localState, localState)
	observeStateSet(c.metrics.ha.peerState, peerState)
	if ha.Group.PeerInfo.ConnStatus != "" {
		observeStateSet(c.metrics.ha.peerConnectionStatus, normalizeUpDownState(ha.Group.PeerInfo.ConnStatus))
	}
	if stateSync != "" {
		observeStateSet(c.metrics.ha.stateSync, normalizeHASyncState(stateSync))
	}
	c.observeHALinkStatus("ha1", ha.Group.PeerInfo.ConnHA1.Status)
	c.observeHALinkStatus("ha1_backup", ha.Group.PeerInfo.ConnHA1B.Status)
	c.observeHALinkStatus("ha2", ha.Group.PeerInfo.ConnHA2.Status)
	c.observeHALinkStatus("ha2_backup", ha.Group.PeerInfo.ConnHA2B.Status)
	return true, nil
}

func (c *Collector) observeHALinkStatus(link, status string) {
	state := normalizeUpDownState(status)
	if state == "" {
		return
	}

	observeStateSetVec(c.metrics.ha.linkStatus, state, link)
}

func parseHAState(body []byte) (haResult, error) {
	var result haResult
	if err := decodePANOSResult(body, "PAN-OS HA response", &result); err != nil {
		return haResult{}, err
	}
	return result, nil
}

func normalizeHAState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	state = strings.ReplaceAll(state, "-", "_")
	state = strings.ReplaceAll(state, " ", "_")
	switch state {
	case "":
		return ""
	case "active", "passive", "suspended", "unknown":
		return state
	case "nonfunctional", "non_functional", "non_function":
		return "non_functional"
	default:
		return "unknown"
	}
}

func (c *Collector) haEnabledStatus(ha haResult) (bool, error) {
	if strings.TrimSpace(ha.Enabled) != "" {
		return parsePANOSAffirmativeField("HA enabled", ha.Enabled)
	}
	return firstNonEmpty(ha.Group.LocalInfo.State, ha.Group.PeerInfo.State, ha.Group.RunningSync) != "", nil
}

func normalizeHASyncState(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "synchronized", "complete":
		return "synchronized"
	case "not synchronized", "not-synchronized", "not_synchronized", "unsynchronized", "out of sync", "out-of-sync", "out_of_sync", "incomplete", "syncing", "synchronizing":
		return "not_synchronized"
	case "":
		return ""
	default:
		return "unknown"
	}
}
