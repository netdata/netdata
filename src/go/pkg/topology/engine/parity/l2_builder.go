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

// BuildOptions configures protocol extraction when converting walk fixtures
// into an engine result.
type BuildOptions struct {
	EnableLLDP   bool
	EnableCDP    bool
	EnableBridge bool
	EnableARP    bool
}

// FixtureWalk is one device fixture with already-parsed walk records.
type FixtureWalk struct {
	DeviceID string
	Hostname string
	Address  string
	Records  []WalkRecord
}

// LoadScenarioWalks loads all walk fixtures referenced by a resolved scenario.
func LoadScenarioWalks(scenario ResolvedScenario) ([]FixtureWalk, error) {
	walks := make([]FixtureWalk, 0, len(scenario.Fixtures))
	for _, fixture := range scenario.Fixtures {
		ds, err := LoadWalkFile(fixture.WalkFile)
		if err != nil {
			return nil, fmt.Errorf("load walk for fixture %q: %w", fixture.DeviceID, err)
		}
		walks = append(walks, FixtureWalk{
			DeviceID: fixture.DeviceID,
			Hostname: fixture.Hostname,
			Address:  fixture.Address,
			Records:  ds.Records,
		})
	}
	return walks, nil
}

// BuildL2ResultFromWalks builds a deterministic layer-2 engine.Result from
// LLDP/CDP/FDB/ARP observations in walk fixtures.
func BuildL2ResultFromWalks(fixtures []FixtureWalk, opts BuildOptions) (engine.Result, error) {
	if len(fixtures) == 0 {
		return engine.Result{}, fmt.Errorf("at least one fixture is required")
	}

	observations := make([]engine.L2Observation, 0, len(fixtures))
	for _, fx := range fixtures {
		if strings.TrimSpace(fx.DeviceID) == "" {
			return engine.Result{}, fmt.Errorf("fixture with empty device id")
		}
		observations = append(observations, parseFixture(fx).toObservation())
	}

	return engine.BuildL2ResultFromObservations(observations, engine.DiscoverOptions{
		EnableLLDP:   opts.EnableLLDP,
		EnableCDP:    opts.EnableCDP,
		EnableBridge: opts.EnableBridge,
		EnableARP:    opts.EnableARP,
	})
}

type parsedFixture struct {
	deviceID            string
	hostname            string
	mgmtIP              string
	sysObjectID         string
	chassisID           string
	ifNameByIndex       map[string]string
	bridgePortToIfIndex map[string]string
	portIDByNum         map[string]string
	portIDSubtypeByNum  map[string]string
	portDescByNum       map[string]string
	fdbEntries          map[string]fdbObs
	arpEntries          map[string]arpObs
	lldpRemotes         map[string]lldpRemoteObs
	cdpRemotes          map[string]cdpRemoteObs
}

type lldpRemoteObs struct {
	localPortNum  string
	remIndex      string
	chassisID     string
	sysName       string
	portID        string
	portIDSubtype string
	portDesc      string
	mgmtIP        string
}

type cdpRemoteObs struct {
	ifIndex     string
	deviceIndex string
	deviceID    string
	devicePort  string
	addressType string
	address     string
}

type fdbObs struct {
	mac        string
	bridgePort string
	status     string
}

type arpObs struct {
	ifIndex  string
	ip       string
	mac      string
	state    string
	addrType string
}

func parseFixture(f FixtureWalk) parsedFixture {
	p := parsedFixture{
		deviceID:            strings.TrimSpace(f.DeviceID),
		hostname:            strings.TrimSpace(f.Hostname),
		mgmtIP:              strings.TrimSpace(f.Address),
		ifNameByIndex:       make(map[string]string),
		bridgePortToIfIndex: make(map[string]string),
		portIDByNum:         make(map[string]string),
		portIDSubtypeByNum:  make(map[string]string),
		portDescByNum:       make(map[string]string),
		fdbEntries:          make(map[string]fdbObs),
		arpEntries:          make(map[string]arpObs),
		lldpRemotes:         make(map[string]lldpRemoteObs),
		cdpRemotes:          make(map[string]cdpRemoteObs),
	}

	for _, rec := range f.Records {
		oid := normalizeOID(rec.OID)
		value := strings.TrimSpace(rec.Value)

		switch {
		case oid == "1.3.6.1.2.1.1.2.0":
			p.sysObjectID = value
		case oid == "1.3.6.1.2.1.1.5.0":
			if p.hostname == "" {
				p.hostname = value
			}
		case oid == "1.0.8802.1.1.2.1.3.3.0":
			p.hostname = value
		case oid == "1.0.8802.1.1.2.1.3.2.0":
			p.chassisID = normalizeHexToken(value)
		case strings.HasPrefix(oid, "1.3.6.1.2.1.31.1.1.1.1."):
			idx := strings.TrimPrefix(oid, "1.3.6.1.2.1.31.1.1.1.1.")
			p.ifNameByIndex[idx] = value
		case strings.HasPrefix(oid, "1.3.6.1.2.1.2.2.1.2."):
			idx := strings.TrimPrefix(oid, "1.3.6.1.2.1.2.2.1.2.")
			if _, ok := p.ifNameByIndex[idx]; !ok {
				p.ifNameByIndex[idx] = value
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.17.1.4.1.2."):
			basePort := strings.TrimPrefix(oid, "1.3.6.1.2.1.17.1.4.1.2.")
			basePort = strings.TrimSpace(basePort)
			if basePort != "" {
				p.bridgePortToIfIndex[basePort] = value
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.17.4.3.1.2."):
			if key, mac, ok := fdbIndexFromOID(oid, "1.3.6.1.2.1.17.4.3.1.2."); ok {
				entry := p.fdbEntries[key]
				if entry.mac == "" {
					entry.mac = mac
				}
				entry.bridgePort = value
				p.fdbEntries[key] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.17.4.3.1.3."):
			if key, mac, ok := fdbIndexFromOID(oid, "1.3.6.1.2.1.17.4.3.1.3."); ok {
				entry := p.fdbEntries[key]
				if entry.mac == "" {
					entry.mac = mac
				}
				entry.status = value
				p.fdbEntries[key] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4.22.1.2."):
			if key, ifIndex, ip, ok := arpLegacyIndex(oid, "1.3.6.1.2.1.4.22.1.2."); ok {
				entry := p.arpEntries[key]
				entry.ifIndex = ifIndex
				entry.ip = ip
				entry.mac = value
				entry.addrType = "ipv4"
				p.arpEntries[key] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4.22.1.3."):
			if key, ifIndex, ip, ok := arpLegacyIndex(oid, "1.3.6.1.2.1.4.22.1.3."); ok {
				entry := p.arpEntries[key]
				entry.ifIndex = ifIndex
				entry.ip = ip
				entry.addrType = "ipv4"
				p.arpEntries[key] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.2.1.4.22.1.4."):
			if key, ifIndex, ip, ok := arpLegacyIndex(oid, "1.3.6.1.2.1.4.22.1.4."); ok {
				entry := p.arpEntries[key]
				entry.ifIndex = ifIndex
				entry.ip = ip
				entry.state = value
				entry.addrType = "ipv4"
				p.arpEntries[key] = entry
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.3."):
			portNum := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.3.")
			p.portIDByNum[portNum] = value
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.2."):
			portNum := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.2.")
			p.portIDSubtypeByNum[portNum] = value
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.4."):
			portNum := strings.TrimPrefix(oid, "1.0.8802.1.1.2.1.3.7.1.4.")
			p.portDescByNum[portNum] = value
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.3.1.1.4."):
			if localPort, ok := nokiaLocalPortIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.3.1.1.4."); ok {
				if portID := normalizeNokiaPortLabel(value); portID != "" {
					if _, exists := p.portIDByNum[localPort]; !exists {
						p.portIDByNum[localPort] = portID
					}
				}
				if strings.TrimSpace(value) != "" {
					if _, exists := p.portDescByNum[localPort]; !exists {
						p.portDescByNum[localPort] = strings.TrimSpace(value)
					}
				}
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.4.1.1.5."):
			if localPort, remIndex, ok := lldpRemoteIndex(oid, "1.0.8802.1.1.2.1.4.1.1.5."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.chassisID = normalizeHexToken(value)
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.4.1.1.7."):
			if localPort, remIndex, ok := lldpRemoteIndex(oid, "1.0.8802.1.1.2.1.4.1.1.7."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portID = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.4.1.1.6."):
			if localPort, remIndex, ok := lldpRemoteIndex(oid, "1.0.8802.1.1.2.1.4.1.1.6."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portIDSubtype = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.4.1.1.8."):
			if localPort, remIndex, ok := lldpRemoteIndex(oid, "1.0.8802.1.1.2.1.4.1.1.8."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portDesc = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.0.8802.1.1.2.1.4.1.1.9."):
			if localPort, remIndex, ok := lldpRemoteIndex(oid, "1.0.8802.1.1.2.1.4.1.1.9."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.sysName = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.5."):
			if localPort, remIndex, ok := nokiaLldpRemoteIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.5."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.chassisID = normalizeHexToken(value)
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.7."):
			if localPort, remIndex, ok := nokiaLldpRemoteIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.7."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portID = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.6."):
			if localPort, remIndex, ok := nokiaLldpRemoteIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.6."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portIDSubtype = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.8."):
			if localPort, remIndex, ok := nokiaLldpRemoteIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.8."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.portDesc = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.9."):
			if localPort, remIndex, ok := nokiaLldpRemoteIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.1.1.9."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.sysName = value
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.2.1.3."):
			if localPort, remIndex, mgmtIP, ok := nokiaLldpRemoteMgmtIndex(oid, "1.3.6.1.4.1.6527.3.1.2.59.4.2.1.3."); ok {
				entry := p.lldpRemote(localPort, remIndex)
				entry.mgmtIP = mgmtIP
				p.lldpRemotes[lldpRemoteKey(localPort, remIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.6."):
			if ifIndex, devIndex, ok := cdpRemoteIndex(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.6."); ok {
				entry := p.cdpRemote(ifIndex, devIndex)
				entry.deviceID = value
				p.cdpRemotes[cdpRemoteKey(ifIndex, devIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.7."):
			if ifIndex, devIndex, ok := cdpRemoteIndex(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.7."); ok {
				entry := p.cdpRemote(ifIndex, devIndex)
				entry.devicePort = value
				p.cdpRemotes[cdpRemoteKey(ifIndex, devIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.4."):
			if ifIndex, devIndex, ok := cdpRemoteIndex(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.4."); ok {
				entry := p.cdpRemote(ifIndex, devIndex)
				entry.address = value
				p.cdpRemotes[cdpRemoteKey(ifIndex, devIndex)] = entry
			}
		case strings.HasPrefix(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.3."):
			if ifIndex, devIndex, ok := cdpRemoteIndex(oid, "1.3.6.1.4.1.9.9.23.1.2.1.1.3."); ok {
				entry := p.cdpRemote(ifIndex, devIndex)
				entry.addressType = value
				p.cdpRemotes[cdpRemoteKey(ifIndex, devIndex)] = entry
			}
		}
	}

	if p.hostname == "" {
		p.hostname = p.deviceID
	}
	return p
}

func (p parsedFixture) toObservation() engine.L2Observation {
	obs := engine.L2Observation{
		DeviceID:     p.deviceID,
		Hostname:     p.hostname,
		ManagementIP: p.mgmtIP,
		SysObjectID:  p.sysObjectID,
		ChassisID:    p.chassisID,
		Interfaces:   p.toInterfaces(),
		BridgePorts:  p.toBridgePorts(),
		FDBEntries:   p.toFDBEntries(),
		ARPNDEntries: p.toARPNDEntries(),
		LLDPRemotes:  p.toLLDPRemotes(),
		CDPRemotes:   p.toCDPRemotes(),
	}
	return obs
}

func (p parsedFixture) toInterfaces() []engine.ObservedInterface {
	if len(p.ifNameByIndex) == 0 {
		return nil
	}
	out := make([]engine.ObservedInterface, 0, len(p.ifNameByIndex))
	for idx, name := range p.ifNameByIndex {
		n, err := strconv.Atoi(idx)
		if err != nil {
			continue
		}
		ifName := strings.TrimSpace(name)
		if ifName == "" {
			continue
		}
		out = append(out, engine.ObservedInterface{IfIndex: n, IfName: ifName, IfDescr: ifName})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].IfIndex != out[j].IfIndex {
			return out[i].IfIndex < out[j].IfIndex
		}
		return out[i].IfName < out[j].IfName
	})
	return out
}

func (p parsedFixture) toBridgePorts() []engine.BridgePortObservation {
	if len(p.bridgePortToIfIndex) == 0 {
		return nil
	}
	out := make([]engine.BridgePortObservation, 0, len(p.bridgePortToIfIndex))
	for basePort, ifIndexRaw := range p.bridgePortToIfIndex {
		ifIndex, err := strconv.Atoi(strings.TrimSpace(ifIndexRaw))
		if err != nil || ifIndex <= 0 {
			continue
		}
		basePort = strings.TrimSpace(basePort)
		if basePort == "" {
			continue
		}
		out = append(out, engine.BridgePortObservation{
			BasePort: basePort,
			IfIndex:  ifIndex,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BasePort != b.BasePort {
			return a.BasePort < b.BasePort
		}
		return a.IfIndex < b.IfIndex
	})
	return out
}

func (p parsedFixture) toFDBEntries() []engine.FDBObservation {
	if len(p.fdbEntries) == 0 {
		return nil
	}
	out := make([]engine.FDBObservation, 0, len(p.fdbEntries))
	for _, entry := range p.fdbEntries {
		mac := normalizeHexToken(entry.mac)
		if mac == "" {
			continue
		}
		out = append(out, engine.FDBObservation{
			MAC:        mac,
			BridgePort: strings.TrimSpace(entry.bridgePort),
			Status:     strings.TrimSpace(entry.status),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.BridgePort != b.BridgePort {
			return a.BridgePort < b.BridgePort
		}
		if a.MAC != b.MAC {
			return a.MAC < b.MAC
		}
		return a.Status < b.Status
	})
	return out
}

func (p parsedFixture) toARPNDEntries() []engine.ARPNDObservation {
	if len(p.arpEntries) == 0 {
		return nil
	}
	out := make([]engine.ARPNDObservation, 0, len(p.arpEntries))
	for _, entry := range p.sortedARPEntries() {
		ifIndex, err := strconv.Atoi(strings.TrimSpace(entry.ifIndex))
		if err != nil {
			ifIndex = 0
		}
		out = append(out, engine.ARPNDObservation{
			Protocol: "arp",
			IfIndex:  ifIndex,
			IfName:   p.ifNameByIndex[entry.ifIndex],
			IP:       strings.TrimSpace(entry.ip),
			MAC:      normalizeHexToken(entry.mac),
			State:    strings.TrimSpace(entry.state),
			AddrType: strings.TrimSpace(entry.addrType),
		})
	}
	return out
}

func (p parsedFixture) toLLDPRemotes() []engine.LLDPRemoteObservation {
	if len(p.lldpRemotes) == 0 {
		return nil
	}
	obs := make([]engine.LLDPRemoteObservation, 0, len(p.lldpRemotes))
	for _, remote := range p.sortedLLDPRemotes() {
		obs = append(obs, engine.LLDPRemoteObservation{
			LocalPortNum:       remote.localPortNum,
			RemoteIndex:        remote.remIndex,
			LocalPortID:        p.portIDByNum[remote.localPortNum],
			LocalPortIDSubtype: p.portIDSubtypeByNum[remote.localPortNum],
			LocalPortDesc:      p.portDescByNum[remote.localPortNum],
			ChassisID:          remote.chassisID,
			SysName:            remote.sysName,
			PortID:             remote.portID,
			PortIDSubtype:      remote.portIDSubtype,
			PortDesc:           remote.portDesc,
			ManagementIP:       remote.mgmtIP,
		})
	}
	return obs
}

func (p parsedFixture) toCDPRemotes() []engine.CDPRemoteObservation {
	if len(p.cdpRemotes) == 0 {
		return nil
	}
	obs := make([]engine.CDPRemoteObservation, 0, len(p.cdpRemotes))
	for _, remote := range p.sortedCDPRemotes() {
		ifIndex, err := strconv.Atoi(remote.ifIndex)
		if err != nil {
			ifIndex = 0
		}
		localName := p.ifNameByIndex[remote.ifIndex]
		obs = append(obs, engine.CDPRemoteObservation{
			LocalIfIndex: ifIndex,
			LocalIfName:  localName,
			DeviceIndex:  remote.deviceIndex,
			DeviceID:     remote.deviceID,
			DevicePort:   remote.devicePort,
			Address:      remote.address,
		})
	}
	return obs
}

func (p parsedFixture) lldpRemote(localPort, remIndex string) lldpRemoteObs {
	key := lldpRemoteKey(localPort, remIndex)
	entry := p.lldpRemotes[key]
	if entry.localPortNum == "" {
		entry.localPortNum = localPort
		entry.remIndex = remIndex
	}
	return entry
}

func (p parsedFixture) cdpRemote(ifIndex, devIndex string) cdpRemoteObs {
	key := cdpRemoteKey(ifIndex, devIndex)
	entry := p.cdpRemotes[key]
	if entry.ifIndex == "" {
		entry.ifIndex = ifIndex
		entry.deviceIndex = devIndex
	}
	return entry
}

func (p parsedFixture) sortedLLDPRemotes() []lldpRemoteObs {
	out := make([]lldpRemoteObs, 0, len(p.lldpRemotes))
	for _, rem := range p.lldpRemotes {
		if rem.chassisID == "" && rem.sysName == "" {
			continue
		}
		out = append(out, rem)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.localPortNum != b.localPortNum {
			return a.localPortNum < b.localPortNum
		}
		if a.remIndex != b.remIndex {
			return a.remIndex < b.remIndex
		}
		if a.sysName != b.sysName {
			return a.sysName < b.sysName
		}
		if a.chassisID != b.chassisID {
			return a.chassisID < b.chassisID
		}
		if a.portID != b.portID {
			return a.portID < b.portID
		}
		if a.portIDSubtype != b.portIDSubtype {
			return a.portIDSubtype < b.portIDSubtype
		}
		if a.portDesc != b.portDesc {
			return a.portDesc < b.portDesc
		}
		return a.mgmtIP < b.mgmtIP
	})
	return out
}

func (p parsedFixture) sortedCDPRemotes() []cdpRemoteObs {
	out := make([]cdpRemoteObs, 0, len(p.cdpRemotes))
	for _, rem := range p.cdpRemotes {
		if rem.deviceID == "" && rem.address == "" {
			continue
		}
		out = append(out, rem)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ifIndex != b.ifIndex {
			return a.ifIndex < b.ifIndex
		}
		if a.deviceIndex != b.deviceIndex {
			return a.deviceIndex < b.deviceIndex
		}
		return a.deviceID < b.deviceID
	})
	return out
}

func (p parsedFixture) sortedARPEntries() []arpObs {
	out := make([]arpObs, 0, len(p.arpEntries))
	for _, entry := range p.arpEntries {
		if strings.TrimSpace(entry.ip) == "" && strings.TrimSpace(entry.mac) == "" {
			continue
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ifIndex != b.ifIndex {
			return a.ifIndex < b.ifIndex
		}
		if a.ip != b.ip {
			return a.ip < b.ip
		}
		if a.mac != b.mac {
			return a.mac < b.mac
		}
		return a.state < b.state
	})
	return out
}

func normalizeHexToken(v string) string {
	if decoded := decodeHexBytes(v); len(decoded) > 0 {
		if len(decoded) == 6 {
			parts := make([]string, 0, 6)
			for _, b := range decoded {
				parts = append(parts, fmt.Sprintf("%02x", b))
			}
			return strings.Join(parts, ":")
		}
		if asIP := decodeHexIP(v); asIP != "" {
			return asIP
		}
	}
	return strings.TrimSpace(v)
}

func decodeHexIP(v string) string {
	bs := decodeHexBytes(v)
	if len(bs) == 4 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.Unmap().String()
		}
	}
	if len(bs) == 16 {
		addr, ok := netip.AddrFromSlice(bs)
		if ok {
			return addr.String()
		}
	}
	return ""
}

func decodeHexBytes(v string) []byte {
	clean := strings.ToLower(strings.TrimSpace(v))
	clean = strings.TrimPrefix(clean, "0x")
	if clean == "" {
		return nil
	}

	if strings.ContainsAny(clean, ":-. \t") {
		parts := strings.FieldsFunc(clean, func(r rune) bool {
			return r == ':' || r == '-' || r == '.' || r == ' ' || r == '\t'
		})
		if len(parts) == 0 {
			return nil
		}

		out := make([]byte, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if len(part) > 2 {
				return nil
			}
			if len(part) == 1 {
				part = "0" + part
			}
			b, err := hex.DecodeString(part)
			if err != nil || len(b) != 1 {
				return nil
			}
			out = append(out, b[0])
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}

	if len(clean)%2 == 1 {
		clean = "0" + clean
	}
	bs, err := hex.DecodeString(clean)
	if err != nil {
		return nil
	}
	return bs
}

func lldpRemoteIndex(oid, prefix string) (localPort string, remIndex string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}

func nokiaLocalPortIndex(oid, prefix string) (localPort string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 2 {
		return "", false
	}
	localPort = strings.TrimSpace(parts[0])
	if localPort == "" {
		return "", false
	}
	return localPort, true
}

func normalizeNokiaPortLabel(v string) string {
	raw := strings.TrimSpace(v)
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, ",", 2)
	label := strings.TrimSpace(parts[0])
	if label != "" {
		return label
	}
	return raw
}

func nokiaLldpRemoteIndex(oid, prefix string) (localPort string, remIndex string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 3 {
		return "", "", false
	}

	localPos := len(parts) - 2
	if len(parts) >= 4 {
		localPos = len(parts) - 3
	}

	localPort = strings.TrimSpace(parts[localPos])
	remIndex = strings.TrimSpace(parts[len(parts)-1])
	if localPort == "" || remIndex == "" {
		return "", "", false
	}
	return localPort, remIndex, true
}

func nokiaLldpRemoteMgmtIndex(oid, prefix string) (localPort string, remIndex string, mgmtIP string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 10 {
		return "", "", "", false
	}

	localPort = strings.TrimSpace(parts[1])
	remIndex = strings.TrimSpace(parts[3])
	if localPort == "" || remIndex == "" {
		return "", "", "", false
	}

	if strings.TrimSpace(parts[len(parts)-5]) != "4" {
		return "", "", "", false
	}

	octets := make([]byte, 0, 4)
	for _, part := range parts[len(parts)-4:] {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", "", "", false
		}
		octets = append(octets, byte(n))
	}

	addr, addrOK := netip.AddrFromSlice(octets)
	if !addrOK {
		return "", "", "", false
	}
	return localPort, remIndex, addr.Unmap().String(), true
}

func cdpRemoteIndex(oid, prefix string) (ifIndex string, deviceIndex string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) < 2 {
		return "", "", false
	}
	return parts[len(parts)-2], parts[len(parts)-1], true
}

func fdbIndexFromOID(oid, prefix string) (key string, mac string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) != 6 {
		return "", "", false
	}

	octets := make([]byte, 0, 6)
	for _, part := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", "", false
		}
		octets = append(octets, byte(n))
	}

	macParts := make([]string, 0, 6)
	for _, octet := range octets {
		macParts = append(macParts, fmt.Sprintf("%02x", octet))
	}

	return strings.Join(parts, "."), strings.Join(macParts, ":"), true
}

func arpLegacyIndex(oid, prefix string) (key string, ifIndex string, ip string, ok bool) {
	suffix := strings.TrimPrefix(oid, prefix)
	suffix = strings.TrimPrefix(suffix, ".")
	parts := strings.Split(suffix, ".")
	if len(parts) != 5 {
		return "", "", "", false
	}

	ifIndex = strings.TrimSpace(parts[0])
	if ifIndex == "" {
		return "", "", "", false
	}

	octets := make([]byte, 0, 4)
	for _, part := range parts[1:] {
		n, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || n < 0 || n > 255 {
			return "", "", "", false
		}
		octets = append(octets, byte(n))
	}
	addr, addrOK := netip.AddrFromSlice(octets)
	if !addrOK {
		return "", "", "", false
	}
	ip = addr.Unmap().String()

	return ifIndex + "|" + ip, ifIndex, ip, true
}

func lldpRemoteKey(localPort, remIndex string) string {
	return localPort + ":" + remIndex
}

func cdpRemoteKey(ifIndex, devIndex string) string {
	return ifIndex + ":" + devIndex
}
