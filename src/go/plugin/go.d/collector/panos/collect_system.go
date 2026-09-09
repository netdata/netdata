// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"fmt"
	"strings"
)

type systemInfo struct {
	Hostname          string `xml:"hostname"`
	DeviceName        string `xml:"devicename"`
	Model             string `xml:"model"`
	Serial            string `xml:"serial"`
	SWVersion         string `xml:"sw-version"`
	Uptime            string `xml:"uptime"`
	CertificateStatus string `xml:"device-certificate-status"`
	OperationalMode   string `xml:"operational-mode"`
}

type systemInfoResult struct {
	System systemInfo `xml:"system"`
}

func (c *Collector) collectSystemMetrics(ctx context.Context) (bool, error) {
	info, err := c.querySystemInfo(ctx)
	if err != nil {
		return false, fmt.Errorf("system metricset: %w", err)
	}

	uptime, err := parseRequiredPANOSDurationField("system uptime", info.Uptime)
	if err != nil {
		return false, fmt.Errorf("system metricset: %s response: %w", panosCommandName(systemInfoCommand), err)
	}

	labels := systemLabelValues(info)
	c.metrics.system.uptime.WithLabelValues(labels...).Observe(float64(uptime))

	certStatus := strings.TrimSpace(info.CertificateStatus)
	if certStatus != "" {
		certValid := strings.EqualFold(certStatus, "valid")
		observeStateSetVec(c.metrics.system.certStatus, boolState(certValid, "valid", "invalid"), labels...)
	}

	operationalMode := strings.TrimSpace(info.OperationalMode)
	if operationalMode != "" {
		normalMode := strings.EqualFold(operationalMode, "normal")
		observeStateSetVec(c.metrics.system.operationalMode, boolState(normalMode, "normal", "other"), labels...)
	}
	return true, nil
}

func (c *Collector) querySystemInfo(ctx context.Context) (systemInfo, error) {
	body, err := c.apiClient.op(ctx, systemInfoCommand)
	if err != nil {
		return systemInfo{}, fmt.Errorf("%s API call: %w", panosCommandName(systemInfoCommand), err)
	}

	info, err := parseSystemInfo(body)
	if err != nil {
		return systemInfo{}, fmt.Errorf("%s response: %w", panosCommandName(systemInfoCommand), err)
	}
	if !info.hasData() {
		return systemInfo{}, fmt.Errorf("%s response: %w", panosCommandName(systemInfoCommand), missingPANOSResultError{expected: "<system>"})
	}
	return info, nil
}

func parseSystemInfo(body []byte) (systemInfo, error) {
	var result systemInfoResult
	if err := decodePANOSResult(body, "PAN-OS system info response", &result); err != nil {
		return systemInfo{}, err
	}
	return result.System, nil
}

func (i systemInfo) hasData() bool {
	return firstNonEmpty(i.Hostname, i.DeviceName, i.Model, i.Serial, i.SWVersion, i.Uptime, i.CertificateStatus, i.OperationalMode) != ""
}
