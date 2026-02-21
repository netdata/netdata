// SPDX-License-Identifier: GPL-3.0-or-later

package parity

import (
	"encoding/hex"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/topology/engine"
)

// OSPFAreaObservation captures one OSPF area row from walk fixtures.
type OSPFAreaObservation struct {
	AreaID          string
	AuthType        int
	ImportAsExtern  int
	AreaBdrRtrCount int
	ASBdrRtrCount   int
	AreaLsaCount    int
}

// BuildL3ResultFromWalks builds a deterministic layer-3 engine.Result from
// OSPF and ISIS observations in walk fixtures.
func BuildL3ResultFromWalks(fixtures []FixtureWalk) (engine.Result, error) {
	if len(fixtures) == 0 {
		return engine.Result{}, fmt.Errorf("at least one fixture is required")
	}

	observations := make([]engine.L3Observation, 0, len(fixtures))
	for _, fx := range fixtures {
		if strings.TrimSpace(fx.DeviceID) == "" {
			return engine.Result{}, fmt.Errorf("fixture with empty device id")
		}
		parsed, err := parseL3Fixture(fx)
		if err != nil {
			return engine.Result{}, err
		}
		observations = append(observations, parsed.Observation)
	}

	return engine.BuildL3ResultFromObservations(observations)
}

// BuildL3ObservationFromWalk parses one fixture into a normalized L3
// observation and OSPF area rows used by tracker-level parity tests.
func BuildL3ObservationFromWalk(fixture FixtureWalk) (engine.L3Observation, []OSPFAreaObservation, error) {
	parsed, err := parseL3Fixture(fixture)
	if err != nil {
		return engine.L3Observation{}, nil, err
	}
	return parsed.Observation, parsed.OSPFAreas, nil
}

type parsedL3Fixture struct {
	Observation engine.L3Observation
	OSPFAreas   []OSPFAreaObservation
}

type ospfAreaRow struct {
	areaID         string
	authType       int
	importAsExtern int
	areaBdrCount   int
	asBdrCount     int
	lsaCount       int
}

type ospfIfRow struct {
	ip               string
	addressLessIndex int
	areaID           string
	netmask          string
}

type ospfNbrRow struct {
	remoteIP       string
	addressLess    int
	remoteRouterID string
}

type isisCircRow struct {
	circIndex  int
	ifIndex    int
	adminState int
}

type isisAdjRow struct {
	circIndex       int
	adjIndex        int
	state           int
	neighborSNPA    string
	neighborSysType int
	neighborSysID   string
	neighborExtCirc int
}

func parseL3Fixture(f FixtureWalk) (parsedL3Fixture, error) {
	if strings.TrimSpace(f.DeviceID) == "" {
		return parsedL3Fixture{}, fmt.Errorf("fixture with empty device id")
	}

	ifNamesByIndex := make(map[string]string)
	ifIndexByIP := make(map[string]int)
	ifMaskByIP := make(map[string]string)
	ospfAreaByKey := make(map[string]*ospfAreaRow)
	ospfIfByKey := make(map[string]*ospfIfRow)
	ospfNbrByKey := make(map[string]*ospfNbrRow)
	isisCircByKey := make(map[string]*isisCircRow)
	isisAdjByKey := make(map[string]*isisAdjRow)

	var (
		hostname       = strings.TrimSpace(f.Hostname)
		mgmtIP         = normalizeRoutingIP(strings.TrimSpace(f.Address))
		sysObjectID    string
		chassisID      string
		ospfRouterID   string
		ospfAdminState int
		ospfVersion    int
		ospfAreaBdr    int
		ospfASBdr      int
		isisSysID      string
		isisAdminState int
	)

	for _, rec := range f.Records {
		oid := normalizeOID(rec.OID)
		value := rec.Value

		switch {
		case oid == "1.3.6.1.2.1.1.2.0":
			sysObjectID = value
		case oid == "1.3.6.1.2.1.1.5.0":
			if hostname == "" {
				hostname = value
			}
		case oid == "1.0.8802.1.1.2.1.3.7.0":
			if hostname == "" {
				hostname = value
			}
		case oid == "1.0.8802.1.1.2.1.3.2.0":
			chassisID = normalizeRoutingHex(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.31.1.1.1.1."):
			ifIndex := suffixFromOID(oid, "1.3.6.1.2.1.31.1.1.1.1.")
			if ifIndex != "" {
				ifNamesByIndex[ifIndex] = strings.TrimSpace(value)
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.2.2.1.2."):
			ifIndex := suffixFromOID(oid, "1.3.6.1.2.1.2.2.1.2.")
			if ifIndex != "" {
				if _, ok := ifNamesByIndex[ifIndex]; !ok {
					ifNamesByIndex[ifIndex] = strings.TrimSpace(value)
				}
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4.20.1.2."):
			ip := normalizeRoutingIP(suffixFromOID(oid, "1.3.6.1.2.1.4.20.1.2."))
			if ip != "" {
				ifIndexByIP[ip] = parseNumeric(value)
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4.20.1.3."):
			ip := normalizeRoutingIP(suffixFromOID(oid, "1.3.6.1.2.1.4.20.1.3."))
			mask := normalizeRoutingIP(value)
			if ip != "" && mask != "" {
				ifMaskByIP[ip] = mask
			}
		case oid == "1.3.6.1.2.1.14.1.1.0":
			ospfRouterID = normalizeRoutingIP(value)
		case oid == "1.3.6.1.2.1.14.1.2.0":
			ospfAdminState = parseNumeric(value)
		case oid == "1.3.6.1.2.1.14.1.3.0":
			ospfVersion = parseNumeric(value)
		case oid == "1.3.6.1.2.1.14.1.4.0":
			ospfAreaBdr = parseNumeric(value)
		case oid == "1.3.6.1.2.1.14.1.5.0":
			ospfASBdr = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.1."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.1.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			if areaID := normalizeRoutingIP(value); areaID != "" {
				row.areaID = areaID
			} else if areaID := normalizeRoutingIP(key); areaID != "" {
				row.areaID = areaID
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.2."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.2.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			row.authType = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.3."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.3.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			row.importAsExtern = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.5."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.5.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			row.areaBdrCount = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.6."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.6.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			row.asBdrCount = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.2.1.7."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.2.1.7.")
			if key == "" {
				continue
			}
			row := ensureOSPFAreaRow(ospfAreaByKey, key)
			row.lsaCount = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.7.1.1."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.7.1.1.")
			if key == "" {
				continue
			}
			row := ensureOSPFIfRow(ospfIfByKey, key)
			row.ip = normalizeRoutingIP(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.7.1.2."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.7.1.2.")
			if key == "" {
				continue
			}
			row := ensureOSPFIfRow(ospfIfByKey, key)
			row.addressLessIndex = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.7.1.3."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.7.1.3.")
			if key == "" {
				continue
			}
			row := ensureOSPFIfRow(ospfIfByKey, key)
			row.areaID = normalizeRoutingIP(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.7.1.4."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.7.1.4.")
			if key == "" {
				continue
			}
			row := ensureOSPFIfRow(ospfIfByKey, key)
			row.netmask = normalizeRoutingIP(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.10.1.1."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.10.1.1.")
			if key == "" {
				continue
			}
			row := ensureOSPFNbrRow(ospfNbrByKey, key)
			row.remoteIP = normalizeRoutingIP(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.10.1.2."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.10.1.2.")
			if key == "" {
				continue
			}
			row := ensureOSPFNbrRow(ospfNbrByKey, key)
			row.addressLess = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.14.10.1.3."):
			key := suffixFromOID(oid, "1.3.6.1.2.1.14.10.1.3.")
			if key == "" {
				continue
			}
			row := ensureOSPFNbrRow(ospfNbrByKey, key)
			row.remoteRouterID = normalizeRoutingIP(value)
		case oid == "1.3.6.1.2.1.138.1.1.1.3.0":
			isisSysID = normalizeRoutingHex(value)
		case oid == "1.3.6.1.2.1.138.1.1.1.8.0":
			isisAdminState = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.3.2.1.2."):
			suffix := suffixFromOID(oid, "1.3.6.1.2.1.138.1.3.2.1.2.")
			circIndex := parseNumeric(suffix)
			if circIndex <= 0 {
				continue
			}
			row := ensureISISCircRow(isisCircByKey, circIndex)
			row.ifIndex = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.3.2.1.3."):
			suffix := suffixFromOID(oid, "1.3.6.1.2.1.138.1.3.2.1.3.")
			circIndex := parseNumeric(suffix)
			if circIndex <= 0 {
				continue
			}
			row := ensureISISCircRow(isisCircByKey, circIndex)
			row.adminState = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.6.1.1.2."):
			circIndex, adjIndex, ok := parseISISAdjIndex(suffixFromOID(oid, "1.3.6.1.2.1.138.1.6.1.1.2."))
			if !ok {
				continue
			}
			row := ensureISISAdjRow(isisAdjByKey, circIndex, adjIndex)
			row.state = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.6.1.1.4."):
			circIndex, adjIndex, ok := parseISISAdjIndex(suffixFromOID(oid, "1.3.6.1.2.1.138.1.6.1.1.4."))
			if !ok {
				continue
			}
			row := ensureISISAdjRow(isisAdjByKey, circIndex, adjIndex)
			row.neighborSNPA = normalizeRoutingHex(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.6.1.1.5."):
			circIndex, adjIndex, ok := parseISISAdjIndex(suffixFromOID(oid, "1.3.6.1.2.1.138.1.6.1.1.5."))
			if !ok {
				continue
			}
			row := ensureISISAdjRow(isisAdjByKey, circIndex, adjIndex)
			row.neighborSysType = parseNumeric(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.6.1.1.6."):
			circIndex, adjIndex, ok := parseISISAdjIndex(suffixFromOID(oid, "1.3.6.1.2.1.138.1.6.1.1.6."))
			if !ok {
				continue
			}
			row := ensureISISAdjRow(isisAdjByKey, circIndex, adjIndex)
			row.neighborSysID = normalizeRoutingHex(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.138.1.6.1.1.7."):
			circIndex, adjIndex, ok := parseISISAdjIndex(suffixFromOID(oid, "1.3.6.1.2.1.138.1.6.1.1.7."))
			if !ok {
				continue
			}
			row := ensureISISAdjRow(isisAdjByKey, circIndex, adjIndex)
			row.neighborExtCirc = parseNumeric(value)
		}
	}

	if hostname == "" {
		hostname = strings.TrimSpace(f.DeviceID)
	}

	observation := engine.L3Observation{
		DeviceID:     strings.TrimSpace(f.DeviceID),
		Hostname:     hostname,
		ManagementIP: mgmtIP,
		SysObjectID:  sysObjectID,
		ChassisID:    chassisID,
		Interfaces:   toRoutingInterfaces(ifNamesByIndex),
	}

	if ospfAdminState == 1 {
		observation.OSPFElement = &engine.OSPFElementObservation{
			RouterID:         ospfRouterID,
			AdminState:       ospfAdminState,
			VersionNumber:    ospfVersion,
			AreaBorderRouter: ospfAreaBdr,
			ASBorderRouter:   ospfASBdr,
		}
	}
	observation.OSPFIfTable = toOSPFIfObservations(ospfIfByKey, ifIndexByIP, ifMaskByIP, ifNamesByIndex)
	observation.OSPFNbrTable = toOSPFNbrObservations(ospfNbrByKey)

	if isisSysID != "" || isisAdminState > 0 {
		observation.ISISElement = &engine.ISISElementObservation{
			DeviceID:   observation.DeviceID,
			SysID:      isisSysID,
			AdminState: isisAdminState,
		}
	}
	observation.ISISCircTable = toISISCircuitObservations(isisCircByKey)
	observation.ISISAdjTable = toISISAdjObservations(isisAdjByKey)

	return parsedL3Fixture{
		Observation: observation,
		OSPFAreas:   toOSPFAreaObservations(ospfAreaByKey),
	}, nil
}

func ensureOSPFAreaRow(rows map[string]*ospfAreaRow, key string) *ospfAreaRow {
	row := rows[key]
	if row == nil {
		row = &ospfAreaRow{}
		rows[key] = row
	}
	return row
}

func ensureOSPFIfRow(rows map[string]*ospfIfRow, key string) *ospfIfRow {
	row := rows[key]
	if row == nil {
		row = &ospfIfRow{}
		rows[key] = row
	}
	return row
}

func ensureOSPFNbrRow(rows map[string]*ospfNbrRow, key string) *ospfNbrRow {
	row := rows[key]
	if row == nil {
		row = &ospfNbrRow{}
		rows[key] = row
	}
	return row
}

func ensureISISCircRow(rows map[string]*isisCircRow, circIndex int) *isisCircRow {
	key := strconv.Itoa(circIndex)
	row := rows[key]
	if row == nil {
		row = &isisCircRow{circIndex: circIndex}
		rows[key] = row
	}
	return row
}

func ensureISISAdjRow(rows map[string]*isisAdjRow, circIndex, adjIndex int) *isisAdjRow {
	key := strconv.Itoa(circIndex) + ":" + strconv.Itoa(adjIndex)
	row := rows[key]
	if row == nil {
		row = &isisAdjRow{
			circIndex: circIndex,
			adjIndex:  adjIndex,
		}
		rows[key] = row
	}
	return row
}

func toRoutingInterfaces(ifNamesByIndex map[string]string) []engine.ObservedInterface {
	if len(ifNamesByIndex) == 0 {
		return nil
	}
	out := make([]engine.ObservedInterface, 0, len(ifNamesByIndex))
	for idx, name := range ifNamesByIndex {
		ifIndex, err := strconv.Atoi(strings.TrimSpace(idx))
		if err != nil || ifIndex <= 0 {
			continue
		}
		ifName := strings.TrimSpace(name)
		if ifName == "" {
			continue
		}
		out = append(out, engine.ObservedInterface{
			IfIndex: ifIndex,
			IfName:  ifName,
			IfDescr: ifName,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].IfName < out[j].IfName
	})
	return out
}

func toOSPFIfObservations(
	rows map[string]*ospfIfRow,
	ifIndexByIP map[string]int,
	ifMaskByIP map[string]string,
	ifNamesByIndex map[string]string,
) []engine.OSPFInterfaceObservation {
	if len(rows) == 0 {
		return nil
	}
	keys := sortedMapKeys(rows)
	out := make([]engine.OSPFInterfaceObservation, 0, len(keys))
	for _, key := range keys {
		row := rows[key]
		if row == nil {
			continue
		}
		localIP := normalizeRoutingIP(row.ip)
		if localIP == "" {
			continue
		}

		ifIndex := row.addressLessIndex
		if ifIndex <= 0 {
			ifIndex = ifIndexByIP[localIP]
		}
		ifName := ""
		if ifIndex > 0 {
			ifName = strings.TrimSpace(ifNamesByIndex[strconv.Itoa(ifIndex)])
		}

		netmask := normalizeRoutingIP(row.netmask)
		if netmask == "" {
			netmask = normalizeRoutingIP(ifMaskByIP[localIP])
		}

		out = append(out, engine.OSPFInterfaceObservation{
			IP:             localIP,
			AddressLessIdx: row.addressLessIndex,
			AreaID:         normalizeRoutingIP(row.areaID),
			IfIndex:        ifIndex,
			IfName:         ifName,
			Netmask:        netmask,
		})
	}
	return out
}

func toOSPFNbrObservations(rows map[string]*ospfNbrRow) []engine.OSPFNeighborObservation {
	if len(rows) == 0 {
		return nil
	}
	keys := sortedMapKeys(rows)
	out := make([]engine.OSPFNeighborObservation, 0, len(keys))
	for _, key := range keys {
		row := rows[key]
		if row == nil {
			continue
		}
		out = append(out, engine.OSPFNeighborObservation{
			RemoteRouterID:       normalizeRoutingIP(row.remoteRouterID),
			RemoteIP:             normalizeRoutingIP(row.remoteIP),
			RemoteAddressLessIdx: row.addressLess,
		})
	}
	return out
}

func toISISCircuitObservations(rows map[string]*isisCircRow) []engine.ISISCircuitObservation {
	if len(rows) == 0 {
		return nil
	}
	keys := sortedMapKeys(rows)
	out := make([]engine.ISISCircuitObservation, 0, len(keys))
	for _, key := range keys {
		row := rows[key]
		if row == nil || row.circIndex <= 0 {
			continue
		}
		out = append(out, engine.ISISCircuitObservation{
			CircIndex:  row.circIndex,
			IfIndex:    row.ifIndex,
			AdminState: row.adminState,
		})
	}
	return out
}

func toISISAdjObservations(rows map[string]*isisAdjRow) []engine.ISISAdjacencyObservation {
	if len(rows) == 0 {
		return nil
	}
	keys := sortedMapKeys(rows)
	out := make([]engine.ISISAdjacencyObservation, 0, len(keys))
	for _, key := range keys {
		row := rows[key]
		if row == nil || row.circIndex <= 0 || row.adjIndex <= 0 {
			continue
		}
		out = append(out, engine.ISISAdjacencyObservation{
			CircIndex:          row.circIndex,
			AdjIndex:           row.adjIndex,
			State:              row.state,
			NeighborSNPA:       row.neighborSNPA,
			NeighborSysType:    row.neighborSysType,
			NeighborSysID:      row.neighborSysID,
			NeighborExtendedID: row.neighborExtCirc,
		})
	}
	return out
}

func toOSPFAreaObservations(rows map[string]*ospfAreaRow) []OSPFAreaObservation {
	if len(rows) == 0 {
		return nil
	}
	keys := sortedMapKeys(rows)
	out := make([]OSPFAreaObservation, 0, len(keys))
	for _, key := range keys {
		row := rows[key]
		if row == nil {
			continue
		}
		areaID := normalizeRoutingIP(row.areaID)
		if areaID == "" {
			areaID = normalizeRoutingIP(key)
		}
		out = append(out, OSPFAreaObservation{
			AreaID:          areaID,
			AuthType:        row.authType,
			ImportAsExtern:  row.importAsExtern,
			AreaBdrRtrCount: row.areaBdrCount,
			ASBdrRtrCount:   row.asBdrCount,
			AreaLsaCount:    row.lsaCount,
		})
	}
	return out
}

func sortedMapKeys[T any](rows map[string]T) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func parseISISAdjIndex(value string) (circIndex int, adjIndex int, ok bool) {
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) < 2 {
		return 0, 0, false
	}
	circIndex = parseNumeric(parts[len(parts)-2])
	adjIndex = parseNumeric(parts[len(parts)-1])
	if circIndex <= 0 || adjIndex <= 0 {
		return 0, 0, false
	}
	return circIndex, adjIndex, true
}

func suffixFromOID(oid, prefix string) string {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	return strings.TrimSpace(suffix)
}

func parseNumeric(v string) int {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		return n
	}

	start := strings.LastIndexByte(v, '(')
	end := strings.LastIndexByte(v, ')')
	if start >= 0 && end > start {
		if n, err := strconv.Atoi(strings.TrimSpace(v[start+1 : end])); err == nil {
			return n
		}
	}

	for i := len(v) - 1; i >= 0; i-- {
		if v[i] < '0' || v[i] > '9' {
			continue
		}
		j := i
		for j >= 0 && v[j] >= '0' && v[j] <= '9' {
			j--
		}
		if n, err := strconv.Atoi(v[j+1 : i+1]); err == nil {
			return n
		}
		break
	}
	return 0
}

func normalizeRoutingIP(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	addr, err := netip.ParseAddr(v)
	if err != nil || !addr.IsValid() {
		return ""
	}
	return addr.Unmap().String()
}

func normalizeRoutingHex(v string) string {
	rawValue := v
	if strings.TrimSpace(rawValue) == "" {
		return ""
	}
	v = strings.TrimSpace(v)
	if asIP := decodeHexIP(v); asIP != "" {
		return asIP
	}

	clean := strings.NewReplacer(":", "", "-", "", ".", "", " ", "", "\t", "").Replace(strings.ToLower(v))
	if clean == "" {
		return ""
	}
	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	if _, err := hex.DecodeString(clean); err != nil {
		if raw := []byte(rawValue); len(raw) == 6 {
			return strings.ToLower(hex.EncodeToString(raw))
		}
		return strings.ToLower(v)
	}
	return clean
}
