// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	systemInfoCommand   = "<show><system><info></info></system></show>"
	haStateCommand      = "<show><high-availability><state></state></high-availability></show>"
	environmentCommand  = "<show><system><environmentals></environmentals></system></show>"
	licenseInfoCommand  = "<request><license><info></info></license></request>"
	ipsecSACommand      = "<show><vpn><ipsec-sa></ipsec-sa></vpn></show>"
	licenseNeverExpires = int64(-1)
)

type panosResultResponse struct {
	XMLName xml.Name             `xml:"response"`
	Status  string               `xml:"status,attr"`
	Code    string               `xml:"code,attr"`
	Message panosResponseMessage `xml:"msg"`
	Result  struct {
		Message  panosResponseMessage `xml:"msg"`
		InnerXML string               `xml:",innerxml"`
	} `xml:"result"`
}

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
	Priority   string `xml:"priority"`
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

type environmentResult struct {
	Thermal     rawXMLSection `xml:"thermal"`
	Fan         rawXMLSection `xml:"fan"`
	Fans        rawXMLSection `xml:"fans"`
	Power       rawXMLSection `xml:"power"`
	PowerSupply rawXMLSection `xml:"power-supply"`
}

type rawXMLSection struct {
	InnerXML string `xml:",innerxml"`
}

type environmentEntry struct {
	Slot        string `xml:"slot"`
	Name        string `xml:"name"`
	Description string `xml:"description"`
	Alarm       string `xml:"alarm"`
	Inserted    string `xml:"Inserted"`
	Min         string `xml:"min"`
	Max         string `xml:"max"`
	DegreesC    string `xml:"DegreesC"`
	RPMs        string `xml:"RPMs"`
	Volts       string `xml:"Volts"`
}

type licenseInfoResult struct {
	Licenses *licenseEntries `xml:"licenses"`
}

type licenseEntries struct {
	Entries []licenseEntry `xml:"entry"`
}

type licenseEntry struct {
	Feature     string `xml:"feature"`
	Description string `xml:"description"`
	Expires     string `xml:"expires"`
	Expired     string `xml:"expired"`
}

type ipsecResult struct {
	NTun    string        `xml:"ntun"`
	Entries *ipsecEntries `xml:"entries"`
}

type ipsecEntries struct {
	Entries []ipsecTunnel `xml:"entry"`
}

type ipsecTunnel struct {
	Name       string `xml:"name"`
	Gateway    string `xml:"gateway"`
	Remote     string `xml:"remote"`
	Protocol   string `xml:"proto"`
	Encryption string `xml:"enc"`
	Remain     string `xml:"remain"`
	TID        string `xml:"tid"`
	ISPI       string `xml:"i_spi"`
	OSPI       string `xml:"o_spi"`
}

func (c *Collector) collectSystemMetrics(mx map[string]int64) error {
	body, err := c.apiClient.op(systemInfoCommand)
	if err != nil {
		return fmt.Errorf("system metricset: %s API call: %w", panosCommandName(systemInfoCommand), err)
	}

	info, err := parseSystemInfo(body)
	if err != nil {
		return fmt.Errorf("system metricset: %s response: %w", panosCommandName(systemInfoCommand), err)
	}
	if !info.hasData() {
		return fmt.Errorf("system metricset: %s response: %w", panosCommandName(systemInfoCommand), missingPANOSResultError{expected: "<system>"})
	}

	c.addSystemCharts(info)

	certStatus := strings.TrimSpace(info.CertificateStatus)
	certValid := strings.EqualFold(certStatus, "valid")
	certInvalid := certStatus != "" && !certValid
	operationalMode := strings.TrimSpace(info.OperationalMode)
	normalMode := strings.EqualFold(operationalMode, "normal")
	otherMode := operationalMode != "" && !normalMode

	uptime, err := parseRequiredPANOSDurationField("system uptime", info.Uptime)
	if err != nil {
		return fmt.Errorf("system metricset: %s response: %w", panosCommandName(systemInfoCommand), err)
	}

	mx["system_uptime"] = uptime
	mx["system_device_certificate_status_valid"] = boolToInt(certValid)
	mx["system_device_certificate_status_invalid"] = boolToInt(certInvalid)
	mx["system_operational_mode_normal"] = boolToInt(normalMode)
	mx["system_operational_mode_other"] = boolToInt(otherMode)
	return nil
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

func (c *Collector) collectHAMetrics(mx map[string]int64) error {
	body, err := c.apiClient.op(haStateCommand)
	if err != nil {
		return fmt.Errorf("ha metricset: %s API call: %w", panosCommandName(haStateCommand), err)
	}

	ha, err := parseHAState(body)
	if err != nil {
		return fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}
	if firstNonEmpty(ha.Enabled, ha.Group.LocalInfo.State, ha.Group.PeerInfo.State) == "" {
		return fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), missingPANOSResultError{expected: "<enabled> or <group>"})
	}

	enabled, err := c.haEnabledStatus(ha)
	if err != nil {
		return fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}
	if !enabled && firstNonEmpty(ha.Group.LocalInfo.State, ha.Group.PeerInfo.State, ha.Group.RunningSync) == "" {
		c.addHAEnabledChart()
		mx["ha_enabled"] = 0
		return nil
	}

	c.addHACharts()

	localState := normalizeHAState(ha.Group.LocalInfo.State)
	peerState := normalizeHAState(ha.Group.PeerInfo.State)
	stateSync := firstNonEmpty(ha.Group.RunningSync, ha.Group.LocalInfo.StateSync)

	mx["ha_enabled"] = boolToInt(enabled)
	for _, state := range haStates {
		mx["ha_local_state_"+state] = boolToInt(localState == state)
		mx["ha_peer_state_"+state] = boolToInt(peerState == state)
	}
	mx["ha_peer_connection_status_up"] = boolToInt(isUp(ha.Group.PeerInfo.ConnStatus))
	mx["ha_state_sync_synchronized"] = boolToInt(isSynchronized(stateSync))
	mx["ha_links_status_ha1"] = boolToInt(isUp(ha.Group.PeerInfo.ConnHA1.Status))
	mx["ha_links_status_ha1_backup"] = boolToInt(isUp(ha.Group.PeerInfo.ConnHA1B.Status))
	mx["ha_links_status_ha2"] = boolToInt(isUp(ha.Group.PeerInfo.ConnHA2.Status))
	mx["ha_links_status_ha2_backup"] = boolToInt(isUp(ha.Group.PeerInfo.ConnHA2B.Status))
	localPriority, err := parseRequiredPANOSIntField("HA local priority", ha.Group.LocalInfo.Priority)
	if err != nil {
		return fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}
	peerPriority, err := parseRequiredPANOSIntField("HA peer priority", ha.Group.PeerInfo.Priority)
	if err != nil {
		return fmt.Errorf("ha metricset: %s response: %w", panosCommandName(haStateCommand), err)
	}

	mx["ha_priority_local"] = localPriority
	mx["ha_priority_peer"] = peerPriority
	return nil
}

func parseHAState(body []byte) (haResult, error) {
	var result haResult
	if err := decodePANOSResult(body, "PAN-OS HA response", &result); err != nil {
		return haResult{}, err
	}
	return result, nil
}

func (c *Collector) collectEnvironmentMetrics(mx map[string]int64) error {
	body, err := c.apiClient.op(environmentCommand)
	if err != nil {
		return fmt.Errorf("environment metricset: %s API call: %w", panosCommandName(environmentCommand), err)
	}

	env, err := parseEnvironment(body)
	if err != nil {
		return fmt.Errorf("environment metricset: %s response: %w", panosCommandName(environmentCommand), err)
	}

	sensors, stats := c.filterEnvironmentSensors(env)
	c.collectEnvironmentSensorCardinalityMetrics(mx, stats)

	var errs []error
	for _, sensor := range sensors {
		entry := sensor.entry
		switch sensor.kind {
		case "temperature":
			key := environmentSensorKey("temperature", entry)
			alarm, err := parsePANOSAlarmField("environment temperature "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				c.addEnvironmentSensorAlarmChart(key, "temperature", entry)
				mx["env_sensor_"+key+"_alarm"] = boolToInt(alarm)
			}
			value, err := parseRequiredPANOSDecimalField("environment temperature "+environmentSensorName(entry), entry.DegreesC, 1000)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			c.addEnvironmentTemperatureChart(key, entry)
			mx["env_temperature_"+key] = value
		case "fan":
			key := environmentSensorKey("fan", entry)
			alarm, err := parsePANOSAlarmField("environment fan "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				c.addEnvironmentSensorAlarmChart(key, "fan", entry)
				mx["env_sensor_"+key+"_alarm"] = boolToInt(alarm)
			}
			value, err := parseRequiredPANOSIntField("environment fan "+environmentSensorName(entry)+" RPMs", entry.RPMs)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			c.addEnvironmentFanChart(key, entry)
			mx["env_fan_"+key+"_speed"] = value
		case "voltage":
			key := environmentSensorKey("voltage", entry)
			alarm, err := parsePANOSAlarmField("environment voltage "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if err != nil {
				errs = append(errs, err)
			} else {
				c.addEnvironmentSensorAlarmChart(key, "voltage", entry)
				mx["env_sensor_"+key+"_alarm"] = boolToInt(alarm)
			}
			value, err := parseRequiredPANOSDecimalField("environment voltage "+environmentSensorName(entry), entry.Volts, 1000)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			c.addEnvironmentVoltageChart(key, entry)
			mx["env_voltage_"+key] = value
		case "power_supply":
			key := environmentSensorKey("power_supply", entry)
			inserted, insertedErr := parsePANOSAffirmativeField("environment power supply "+environmentSensorName(entry)+" inserted", entry.Inserted)
			if insertedErr != nil {
				errs = append(errs, insertedErr)
			}
			alarm, alarmErr := parsePANOSAlarmField("environment power supply "+environmentSensorName(entry)+" alarm", entry.Alarm)
			if alarmErr != nil {
				errs = append(errs, alarmErr)
			}
			if insertedErr == nil && alarmErr == nil {
				c.addEnvironmentPowerSupplyChart(key, entry)
				mx["env_psu_"+key+"_inserted"] = boolToInt(inserted)
				mx["env_psu_"+key+"_alarm"] = boolToInt(alarm)
			}
		}
	}

	return errors.Join(errs...)
}

type environmentMetrics struct {
	ThermalEntries     []environmentEntry
	FanEntries         []environmentEntry
	VoltageEntries     []environmentEntry
	PowerSupplyEntries []environmentEntry
}

func parseEnvironment(body []byte) (environmentMetrics, error) {
	var result environmentResult
	if err := decodePANOSResult(body, "PAN-OS environment response", &result); err != nil {
		return environmentMetrics{}, err
	}
	if !result.hasAnySection() {
		return environmentMetrics{}, missingPANOSResultError{expected: "<thermal>, <fan>, <fans>, <power>, or <power-supply>"}
	}

	thermal, err := decodeEnvironmentEntries(result.Thermal.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("thermal entries: %w", err)
	}
	fan, err := decodeEnvironmentEntries(firstNonEmpty(result.Fan.InnerXML, result.Fans.InnerXML))
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("fan entries: %w", err)
	}
	voltage, err := decodeEnvironmentEntries(result.Power.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("voltage entries: %w", err)
	}
	psu, err := decodeEnvironmentEntries(result.PowerSupply.InnerXML)
	if err != nil {
		return environmentMetrics{}, fmt.Errorf("power supply entries: %w", err)
	}

	return environmentMetrics{
		ThermalEntries:     thermal,
		FanEntries:         fan,
		VoltageEntries:     voltage,
		PowerSupplyEntries: psu,
	}, nil
}

func (r environmentResult) hasAnySection() bool {
	return firstNonEmpty(r.Thermal.InnerXML, r.Fan.InnerXML, r.Fans.InnerXML, r.Power.InnerXML, r.PowerSupply.InnerXML) != ""
}

func decodeEnvironmentEntries(innerXML string) ([]environmentEntry, error) {
	if strings.TrimSpace(innerXML) == "" {
		return nil, nil
	}

	decoder := xml.NewDecoder(strings.NewReader(innerXML))
	var entries []environmentEntry

	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return entries, nil
			}
			return nil, err
		}

		start, ok := tok.(xml.StartElement)
		if !ok || start.Name.Local != "entry" {
			continue
		}

		var entry environmentEntry
		if err := decoder.DecodeElement(&entry, &start); err != nil {
			return nil, err
		}
		if firstNonEmpty(entry.Description, entry.Name, entry.Slot) == "" {
			continue
		}
		entries = append(entries, entry)
	}
}

func (c *Collector) collectLicenseMetrics(mx map[string]int64) error {
	body, err := c.apiClient.op(licenseInfoCommand)
	if err != nil {
		return fmt.Errorf("licenses metricset: %s API call: %w", panosCommandName(licenseInfoCommand), err)
	}

	licenses, found, err := parseLicenses(body)
	if err != nil {
		return fmt.Errorf("licenses metricset: %s response: %w", panosCommandName(licenseInfoCommand), err)
	}
	if !found {
		return fmt.Errorf("licenses metricset: %s response: %w", panosCommandName(licenseInfoCommand), missingPANOSResultError{expected: "<licenses>"})
	}

	c.addLicenseCountChart()

	var expired int64
	var errs []error
	for _, entry := range licenses {
		isExpired, err := c.licenseExpiredStatus(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("license %s expired status: %w", firstNonEmpty(entry.Feature, "unknown"), err))
			continue
		}
		if isExpired {
			expired++
		}
	}

	monitoredLicenses, stats := c.filterLicenses(licenses)
	c.collectLicenseCardinalityMetrics(mx, stats)

	for _, entry := range monitoredLicenses {
		key := licenseKey(entry)
		isExpired, err := c.licenseExpiredStatus(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("license %s expired status: %w", firstNonEmpty(entry.Feature, "unknown"), err))
		} else {
			c.addLicenseStatusChart(key, entry)
			mx["license_"+key+"_status_valid"] = boolToInt(!isExpired)
			mx["license_"+key+"_status_expired"] = boolToInt(isExpired)
		}
		days, err := c.licenseDaysUntilExpiration(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("license %s expiration: %w", firstNonEmpty(entry.Feature, "unknown"), err))
			continue
		}
		c.addLicenseExpirationChart(key, entry)
		mx["license_"+key+"_days_until_expiration"] = days
	}

	mx["license_count_total"] = int64(len(licenses))
	mx["license_count_expired"] = expired
	return errors.Join(errs...)
}

func parseLicenses(body []byte) ([]licenseEntry, bool, error) {
	var result licenseInfoResult
	if err := decodePANOSResult(body, "PAN-OS licenses response", &result); err != nil {
		return nil, false, err
	}
	if result.Licenses == nil {
		return nil, false, nil
	}
	return result.Licenses.Entries, true, nil
}

func (c *Collector) collectIPSecMetrics(mx map[string]int64) error {
	body, err := c.apiClient.op(ipsecSACommand)
	if err != nil {
		return fmt.Errorf("ipsec metricset: %s API call: %w", panosCommandName(ipsecSACommand), err)
	}

	tunnels, activeCount, found, err := parseIPSecTunnels(body)
	if err != nil {
		return fmt.Errorf("ipsec metricset: %s response: %w", panosCommandName(ipsecSACommand), err)
	}
	if !found {
		return fmt.Errorf("ipsec metricset: %s response: %w", panosCommandName(ipsecSACommand), missingPANOSResultError{expected: "<ntun> or <entries>"})
	}

	c.addIPSecTunnelsChart()
	mx["ipsec_tunnels_active"] = activeCount

	monitoredTunnels, stats := c.filterIPSecTunnels(tunnels)
	c.collectIPSecTunnelCardinalityMetrics(mx, stats)

	var errs []error
	if activeCount != int64(len(tunnels)) {
		errs = append(errs, fmt.Errorf("IPsec active tunnel count mismatch: ntun=%d entries=%d; per-tunnel lifetime metrics may be incomplete", activeCount, len(tunnels)))
	}
	for _, tunnel := range monitoredTunnels {
		key := ipsecTunnelKey(tunnel)
		value, err := parseRequiredPANOSIntField("IPsec tunnel "+firstNonEmpty(tunnel.Name, key)+" remain", tunnel.Remain)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		c.addIPSecTunnelCharts(key, tunnel)
		mx["ipsec_tunnel_"+key+"_sa_lifetime"] = value
	}
	return errors.Join(errs...)
}

func parseIPSecTunnels(body []byte) ([]ipsecTunnel, int64, bool, error) {
	var result ipsecResult
	if err := decodePANOSResult(body, "PAN-OS IPsec response", &result); err != nil {
		return nil, 0, false, err
	}
	if result.Entries == nil && strings.TrimSpace(result.NTun) == "" {
		return nil, 0, false, nil
	}

	var activeCount int64
	if strings.TrimSpace(result.NTun) != "" {
		count, err := parseRequiredPANOSIntField("IPsec active tunnel count", result.NTun)
		if err != nil {
			return nil, 0, true, err
		}
		activeCount = count
	}
	if result.Entries == nil {
		return nil, activeCount, true, nil
	}
	if strings.TrimSpace(result.NTun) == "" {
		activeCount = int64(len(result.Entries.Entries))
	}
	return result.Entries.Entries, activeCount, true, nil
}

func decodePANOSResult(body []byte, context string, dst any) error {
	innerXML, err := decodePANOSResultInner(body, context)
	if err != nil {
		return err
	}
	if strings.TrimSpace(innerXML) == "" || dst == nil {
		return nil
	}

	wrapped := []byte("<result>" + innerXML + "</result>")
	if err := xml.Unmarshal(wrapped, dst); err != nil {
		return fmt.Errorf("parse %s result: %w", context, err)
	}
	return nil
}

func decodePANOSResultInner(body []byte, context string) (string, error) {
	var resp panosResultResponse
	if err := xml.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("parse %s: %w", context, err)
	}
	if resp.failed() {
		return "", panosResponseError{code: resp.Code, message: resp.errorMessage()}
	}
	return resp.Result.InnerXML, nil
}

func (r panosResultResponse) failed() bool {
	status := strings.ToLower(strings.TrimSpace(r.Status))
	if status == "error" || status == "failed" {
		return true
	}
	code := strings.TrimSpace(r.Code)
	if code == "" || code == "0" || code == "19" || code == "20" {
		return false
	}
	return true
}

func (r panosResultResponse) errorMessage() string {
	return firstNonEmpty(r.Message.String(), r.Result.Message.String(), panosResponseCodeName(r.Code))
}

func normalizeHAState(state string) string {
	state = strings.ToLower(strings.TrimSpace(state))
	state = strings.ReplaceAll(state, "-", "_")
	state = strings.ReplaceAll(state, " ", "_")
	switch state {
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

func parsePANOSAffirmativeField(field, v string) (bool, error) {
	raw := strings.TrimSpace(v)
	switch strings.ToLower(raw) {
	case "yes", "true", "enabled", "enable", "up", "valid":
		return true, nil
	case "no", "false", "disabled", "disable", "down", "invalid", "off", "absent", "not present", "not-present":
		return false, nil
	default:
		if raw == "" {
			return false, fmt.Errorf("%s: missing status", field)
		}
		return false, fmt.Errorf("%s: invalid status %q", field, raw)
	}
}

func isUp(v string) bool {
	return strings.EqualFold(strings.TrimSpace(v), "up")
}

func isSynchronized(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "synchronized", "complete":
		return true
	default:
		return false
	}
}

func parsePANOSAlarmField(field, v string) (bool, error) {
	raw := strings.TrimSpace(v)
	switch strings.ToLower(raw) {
	case "true", "yes", "on", "active", "alarm", "critical":
		return true, nil
	case "false", "no", "off", "inactive", "ok", "normal", "clear", "none":
		return false, nil
	default:
		if raw == "" {
			return false, fmt.Errorf("%s: missing status", field)
		}
		return false, fmt.Errorf("%s: invalid status %q", field, raw)
	}
}

func (c *Collector) licenseExpiredStatus(entry licenseEntry) (bool, error) {
	raw := strings.TrimSpace(entry.Expired)
	switch strings.ToLower(raw) {
	case "yes", "true", "expired":
		return true, nil
	case "no", "false", "valid":
		return false, nil
	case "":
		expires := strings.TrimSpace(entry.Expires)
		if strings.EqualFold(expires, "never") {
			return false, nil
		}
		if expires == "" {
			return false, errors.New("missing status")
		}
		exp, err := parseLicenseExpirationDate(expires)
		if err != nil {
			return false, fmt.Errorf("missing status and %w", err)
		}
		now := c.now().UTC()
		today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		expireDay := time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, time.UTC)
		return expireDay.Before(today), nil
	default:
		return false, fmt.Errorf("invalid status %q", raw)
	}
}

func parsePANOSDecimalField(field, v string, scale int64) (int64, error) {
	raw := strings.TrimSpace(v)
	v = strings.ReplaceAll(raw, ",", "")
	if v == "" {
		return 0, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("%s: invalid decimal %q", field, raw)
	}
	return int64(math.Round(f * float64(scale))), nil
}

func parseRequiredPANOSDecimalField(field, v string, scale int64) (int64, error) {
	if strings.TrimSpace(v) == "" {
		return 0, fmt.Errorf("%s: missing decimal", field)
	}
	return parsePANOSDecimalField(field, v, scale)
}

type missingPANOSResultError struct {
	expected string
}

func (e missingPANOSResultError) Error() string {
	return fmt.Sprintf("PAN-OS XML API success response has no recognized telemetry payload; expected %s", e.expected)
}

func boolToInt(ok bool) int64 {
	if ok {
		return 1
	}
	return 0
}

func environmentSensorKey(kind string, entry environmentEntry) string {
	return cleanID(kind + "_" + firstNonEmpty(entry.Slot, "unknown") + "_" + firstNonEmpty(entry.Description, entry.Name, "unknown"))
}

func environmentSensorName(entry environmentEntry) string {
	return firstNonEmpty(entry.Description, entry.Name, "unknown")
}

func licenseKey(entry licenseEntry) string {
	return cleanID(entry.Feature)
}

func (c *Collector) licenseDaysUntilExpiration(entry licenseEntry) (int64, error) {
	expires := strings.TrimSpace(entry.Expires)
	if strings.EqualFold(expires, "never") {
		return licenseNeverExpires, nil
	}
	if expires == "" {
		return 0, errors.New("missing expiration date")
	}
	exp, err := parseLicenseExpirationDate(expires)
	if err != nil {
		return 0, err
	}

	now := c.now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	expireDay := time.Date(exp.Year(), exp.Month(), exp.Day(), 0, 0, 0, 0, time.UTC)
	days := int64(expireDay.Sub(today).Hours() / 24)
	if days < 0 {
		return 0, nil
	}
	return days, nil
}

func parseLicenseExpirationDate(expires string) (time.Time, error) {
	exp, err := time.ParseInLocation("January 02, 2006", expires, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid expiration date %q", expires)
	}
	return exp, nil
}

func ipsecTunnelKey(tunnel ipsecTunnel) string {
	return cleanID(firstNonEmpty(tunnel.Name, "unknown") + "_" + tunnel.Gateway + "_" + tunnel.Remote + "_" + firstNonEmpty(tunnel.TID, tunnel.ISPI, tunnel.OSPI))
}

func panosCommandName(cmd string) string {
	switch cmd {
	case systemInfoCommand:
		return "system info query"
	case haStateCommand:
		return "HA state query"
	case environmentCommand:
		return "environmentals query"
	case licenseInfoCommand:
		return "license info query"
	case ipsecSACommand:
		return "IPsec SA query"
	default:
		return bgpCommandName(cmd)
	}
}

var haStates = []string{"active", "passive", "non_functional", "suspended", "unknown"}
