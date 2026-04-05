// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	birdNow = time.Now

	birdProtocolHeaderPattern = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\d{4}-\d{2}-\d{2}(?: \d{2}:\d{2}:\d{2}(?:\.\d+)?)?|[^\s]+)(?:\s+(.*))?$`)
	birdChannelPattern        = regexp.MustCompile(`^\s*Channel\s+(.+?)\s*$`)
	birdRoutesPattern         = regexp.MustCompile(`^\s*Routes:\s+(\d+)\s+imported(?:,\s+(\d+)\s+filtered)?(?:,\s+(\d+)\s+exported)?(?:,\s+(\d+)\s+preferred)?\s*$`)
)

type birdProtocol struct {
	Name        string
	Proto       string
	Table       string
	Status      string
	Info        string
	Description string
	BGPState    string
	PeerAddress string
	RemoteAS    int64
	LocalAS     int64
	UptimeSecs  int64
	Channels    []birdChannel
}

type birdChannel struct {
	Name      string
	Table     string
	Imported  int64
	Filtered  int64
	Exported  int64
	Preferred int64

	ImportUpdates   birdRouteChangeCount
	ImportWithdraws birdRouteChangeCount
	ExportUpdates   birdRouteChangeCount
	ExportWithdraws birdRouteChangeCount
}

type birdRouteChangeCount struct {
	Received int64
	Rejected int64
	Filtered int64
	Ignored  int64
	Accepted int64
}

func parseBIRDProtocolsAll(data []byte) ([]birdProtocol, error) {
	lines := normalizeBIRDReplyLines(data)
	protocols := make([]birdProtocol, 0)

	var current *birdProtocol
	var currentChannel *birdChannel

	flushChannel := func() {
		if current == nil || currentChannel == nil {
			return
		}
		current.Channels = append(current.Channels, *currentChannel)
		currentChannel = nil
	}

	flushProtocol := func() {
		if current == nil {
			return
		}
		flushChannel()
		protocols = append(protocols, *current)
		current = nil
	}

	for _, line := range lines {
		if line == "" {
			continue
		}

		if proto, ok, err := parseBIRDProtocolHeader(line); err != nil {
			return nil, err
		} else if ok {
			flushProtocol()
			current = &proto
			continue
		}

		if current == nil {
			continue
		}

		if channel, ok := parseBIRDChannelHeader(line); ok {
			flushChannel()
			currentChannel = &birdChannel{Name: channel}
			continue
		}

		field, value, ok := parseBIRDField(line)
		if ok {
			assignBIRDField(current, &currentChannel, field, value)
			continue
		}

		if routes, ok := parseBIRDRoutes(line); ok {
			channel := ensureBIRDChannel(current, &currentChannel)
			channel.Imported = routes.Imported
			channel.Filtered = routes.Filtered
			channel.Exported = routes.Exported
			channel.Preferred = routes.Preferred
			continue
		}

		if change, ok := parseBIRDRouteChange(line); ok {
			channel := ensureBIRDChannel(current, &currentChannel)
			assignBIRDRouteChange(channel, change)
			continue
		}
	}

	flushProtocol()
	return protocols, nil
}

func normalizeBIRDReplyLines(data []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	lines := make([]string, 0)

	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), " \r")
		cleaned, keep := cleanBIRDReplyLine(line)
		if !keep {
			continue
		}
		lines = append(lines, cleaned)
	}

	return lines
}

func cleanBIRDReplyLine(line string) (string, bool) {
	switch {
	case line == "":
		return "", true
	case strings.HasPrefix(line, " "), strings.HasPrefix(line, "+"):
		return line[1:], true
	}

	match := birdReplyLinePattern.FindStringSubmatch(line)
	if match == nil {
		return line, true
	}

	code, _ := strconv.Atoi(match[1])
	if code < 1000 {
		return "", false
	}

	return match[3], true
}

func parseBIRDProtocolHeader(line string) (birdProtocol, bool, error) {
	match := birdProtocolHeaderPattern.FindStringSubmatch(line)
	if match == nil {
		return birdProtocol{}, false, nil
	}
	if strings.EqualFold(strings.TrimSpace(match[1]), "name") &&
		strings.EqualFold(strings.TrimSpace(match[2]), "proto") &&
		strings.EqualFold(strings.TrimSpace(match[3]), "table") {
		return birdProtocol{}, false, nil
	}

	uptime, err := parseBIRDUptime(match[5], birdNow())
	if err != nil {
		return birdProtocol{}, false, err
	}

	return birdProtocol{
		Name:       strings.TrimSpace(match[1]),
		Proto:      strings.TrimSpace(match[2]),
		Table:      strings.TrimSpace(match[3]),
		Status:     strings.TrimSpace(match[4]),
		UptimeSecs: uptime,
		Info:       strings.TrimSpace(match[6]),
	}, true, nil
}

func parseBIRDChannelHeader(line string) (string, bool) {
	match := birdChannelPattern.FindStringSubmatch(line)
	if match == nil {
		return "", false
	}
	return normalizeBIRDChannelName(match[1]), true
}

func parseBIRDField(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "Description:"):
		return "description", strings.TrimSpace(strings.TrimPrefix(line, "Description:")), true
	case strings.HasPrefix(line, "BGP state:"):
		return "bgp_state", strings.TrimSpace(strings.TrimPrefix(line, "BGP state:")), true
	case strings.HasPrefix(line, "Neighbor address:"):
		return "peer_address", strings.TrimSpace(strings.TrimPrefix(line, "Neighbor address:")), true
	case strings.HasPrefix(line, "Neighbor range:"):
		return "peer_address", strings.TrimSpace(strings.TrimPrefix(line, "Neighbor range:")), true
	case strings.HasPrefix(line, "Neighbor AS:"):
		return "remote_as", strings.TrimSpace(strings.TrimPrefix(line, "Neighbor AS:")), true
	case strings.HasPrefix(line, "Local AS:"):
		return "local_as", strings.TrimSpace(strings.TrimPrefix(line, "Local AS:")), true
	case strings.HasPrefix(line, "Table:"):
		return "table", strings.TrimSpace(strings.TrimPrefix(line, "Table:")), true
	default:
		return "", "", false
	}
}

type birdRoutes struct {
	Imported  int64
	Filtered  int64
	Exported  int64
	Preferred int64
}

func parseBIRDRoutes(line string) (birdRoutes, bool) {
	match := birdRoutesPattern.FindStringSubmatch(line)
	if match == nil {
		return birdRoutes{}, false
	}

	return birdRoutes{
		Imported:  parseBIRDCount(match[1]),
		Filtered:  parseBIRDCount(match[2]),
		Exported:  parseBIRDCount(match[3]),
		Preferred: parseBIRDCount(match[4]),
	}, true
}

type birdRouteChangeLine struct {
	Direction string
	Kind      string
	Counts    birdRouteChangeCount
}

func parseBIRDRouteChange(line string) (birdRouteChangeLine, bool) {
	line = strings.TrimSpace(line)
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return birdRouteChangeLine{}, false
	}

	header := strings.Fields(strings.TrimSpace(parts[0]))
	if len(header) != 2 {
		return birdRouteChangeLine{}, false
	}
	direction := strings.ToLower(strings.TrimSpace(header[0]))
	kind := strings.ToLower(strings.TrimSpace(header[1]))
	if (direction != "import" && direction != "export") || (kind != "updates" && kind != "withdraws") {
		return birdRouteChangeLine{}, false
	}

	fields := strings.Fields(strings.TrimSpace(parts[1]))
	if len(fields) < 5 {
		return birdRouteChangeLine{}, false
	}

	return birdRouteChangeLine{
		Direction: direction,
		Kind:      kind,
		Counts: birdRouteChangeCount{
			Received: parseBIRDCount(fields[0]),
			Rejected: parseBIRDCount(fields[1]),
			Filtered: parseBIRDCount(fields[2]),
			Ignored:  parseBIRDCount(fields[3]),
			Accepted: parseBIRDCount(fields[len(fields)-1]),
		},
	}, true
}

func assignBIRDField(protocol *birdProtocol, currentChannel **birdChannel, field, value string) {
	switch field {
	case "description":
		protocol.Description = strings.TrimSpace(value)
	case "bgp_state":
		protocol.BGPState = strings.TrimSpace(value)
	case "peer_address":
		protocol.PeerAddress = normalizeBIRDPeerAddress(value)
	case "remote_as":
		protocol.RemoteAS = parseBIRDCount(value)
	case "local_as":
		protocol.LocalAS = parseBIRDCount(value)
	case "table":
		channel := ensureBIRDChannel(protocol, currentChannel)
		channel.Table = strings.TrimSpace(value)
	}
}

func ensureBIRDChannel(protocol *birdProtocol, currentChannel **birdChannel) *birdChannel {
	if *currentChannel == nil {
		*currentChannel = &birdChannel{Name: inferBIRDImplicitChannel(protocol)}
	}
	return *currentChannel
}

func assignBIRDRouteChange(channel *birdChannel, change birdRouteChangeLine) {
	switch {
	case change.Direction == "import" && change.Kind == "updates":
		channel.ImportUpdates = change.Counts
	case change.Direction == "import" && change.Kind == "withdraws":
		channel.ImportWithdraws = change.Counts
	case change.Direction == "export" && change.Kind == "updates":
		channel.ExportUpdates = change.Counts
	case change.Direction == "export" && change.Kind == "withdraws":
		channel.ExportWithdraws = change.Counts
	}
}

func normalizeBIRDPeerAddress(value string) string {
	fields := strings.Fields(strings.TrimSpace(value))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func inferBIRDImplicitChannel(protocol *birdProtocol) string {
	switch {
	case strings.Contains(protocol.PeerAddress, ":"):
		return "ipv6"
	case strings.TrimSpace(protocol.PeerAddress) != "":
		return "ipv4"
	}

	table := strings.ToLower(strings.TrimSpace(protocol.Table))
	switch {
	case strings.HasSuffix(table, "6"):
		return "ipv6"
	case strings.HasSuffix(table, "4"):
		return "ipv4"
	default:
		return "unknown"
	}
}

func parseBIRDUptime(value string, now time.Time) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}

	if unix, err := strconv.ParseInt(value, 10, 64); err == nil {
		return maxDurationSeconds(now.Unix() - unix), nil
	}

	if parts := strings.Split(value, ":"); len(parts) == 3 && !strings.Contains(value, " ") {
		secPart := parts[2]
		if idx := strings.Index(secPart, "."); idx >= 0 {
			secPart = secPart[:idx]
		}
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, err
		}
		minutes, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, err
		}
		seconds, err := strconv.Atoi(secPart)
		if err != nil {
			return 0, err
		}
		return int64(hours*3600 + minutes*60 + seconds), nil
	}

	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if ts, err := time.ParseInLocation(layout, value, now.Location()); err == nil {
			return maxDurationSeconds(int64(now.Sub(ts).Seconds())), nil
		}
	}

	return 0, fmt.Errorf("unsupported BIRD uptime %q", value)
}

func parseBIRDCount(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "---" {
		return 0
	}

	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return count
}

func maxDurationSeconds(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}
