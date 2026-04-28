// SPDX-License-Identifier: GPL-3.0-or-later

package engine

import (
	"net/netip"
	"strings"
)

func (s *l2BuildState) isMACCompatibleWithDevice(deviceID, remoteMAC string) bool {
	deviceID = strings.TrimSpace(deviceID)
	remoteMAC = normalizeMAC(remoteMAC)
	if deviceID == "" || remoteMAC == "" {
		return true
	}
	device, ok := s.devices[deviceID]
	if !ok {
		return true
	}
	deviceMAC := primaryL2MACIdentity(device.ChassisID, "")
	if deviceMAC == "" {
		return true
	}
	return deviceMAC == remoteMAC
}

func (s *l2BuildState) shouldEnforceHostnameMACGuard(deviceID, mgmtIP string) bool {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return false
	}
	if canonicalIP(mgmtIP) != "" {
		return true
	}
	return s.managedObservationByDeviceID[deviceID]
}

func (s *l2BuildState) resolveKnownRemote(hostname, chassisID, mgmtIP, remoteMAC string) string {
	remoteIP := canonicalIP(mgmtIP)
	enforceMACGuard := remoteMAC != ""
	candidates := []string{
		s.hostToID[canonicalHost(hostname)],
		s.chassisToID[canonicalToken(chassisID)],
		s.ipToID[remoteIP],
	}
	for _, candidateID := range candidates {
		candidateID = strings.TrimSpace(candidateID)
		if candidateID == "" {
			continue
		}
		if enforceMACGuard && !s.isMACCompatibleWithDevice(candidateID, remoteMAC) {
			continue
		}
		return candidateID
	}
	return ""
}

func (s *l2BuildState) resolveRemote(hostname, chassisID, mgmtIP, fallbackID string) string {
	return s.resolveRemoteWithHostnameMACGuard(hostname, chassisID, mgmtIP, fallbackID, false)
}

func (s *l2BuildState) resolveRemoteEnforcingHostnameMACGuard(hostname, chassisID, mgmtIP, fallbackID string) string {
	return s.resolveRemoteWithHostnameMACGuard(hostname, chassisID, mgmtIP, fallbackID, true)
}

func (s *l2BuildState) resolveRemoteWithHostnameMACGuard(hostname, chassisID, mgmtIP, fallbackID string, enforceHostnameMACGuard bool) string {
	remoteMAC := primaryL2MACIdentity(chassisID, "")
	if knownID := s.resolveKnownRemote(hostname, chassisID, mgmtIP, remoteMAC); knownID != "" {
		if remoteMAC != "" {
			s.macToID[remoteMAC] = knownID
			s.chassisToID[canonicalToken(remoteMAC)] = knownID
			if device, ok := s.devices[knownID]; ok && primaryL2MACIdentity(device.ChassisID, "") == "" {
				device.ChassisID = remoteMAC
				s.devices[knownID] = device
			}
		}
		return knownID
	}

	if remoteMAC != "" {
		if id := s.macToID[remoteMAC]; id != "" {
			return id
		}

		generatedID := deriveRemoteDeviceID(hostname, remoteMAC, mgmtIP, fallbackID)
		if existingID := strings.TrimSpace(s.hostToID[canonicalHost(hostname)]); existingID != "" &&
			(enforceHostnameMACGuard || s.shouldEnforceHostnameMACGuard(existingID, mgmtIP)) &&
			!s.isMACCompatibleWithDevice(existingID, remoteMAC) {
			generatedID = deriveRemoteDeviceID("", remoteMAC, mgmtIP, fallbackID)
		}
		if _, ok := s.devices[generatedID]; !ok {
			device := Device{
				ID:        generatedID,
				Hostname:  strings.TrimSpace(hostname),
				SysObject: "",
				ChassisID: remoteMAC,
			}
			if device.Hostname == "" {
				device.Hostname = generatedID
			}
			if ip := parseAddr(mgmtIP); ip.IsValid() {
				device.Addresses = []netip.Addr{ip}
			}
			s.devices[generatedID] = device
		}

		s.macToID[remoteMAC] = generatedID
		s.chassisToID[canonicalToken(remoteMAC)] = generatedID
		if host := canonicalHost(hostname); host != "" {
			if existingID := strings.TrimSpace(s.hostToID[host]); existingID == "" ||
				(!enforceHostnameMACGuard && !s.shouldEnforceHostnameMACGuard(existingID, mgmtIP)) ||
				s.isMACCompatibleWithDevice(existingID, remoteMAC) {
				s.hostToID[host] = generatedID
			}
		}
		if ip := canonicalIP(mgmtIP); ip != "" {
			if existingID := strings.TrimSpace(s.ipToID[ip]); existingID == "" || s.isMACCompatibleWithDevice(existingID, remoteMAC) {
				s.ipToID[ip] = generatedID
			}
		}
		return generatedID
	}

	if id := s.hostToID[canonicalHost(hostname)]; id != "" {
		return id
	}
	if id := s.chassisToID[canonicalToken(chassisID)]; id != "" {
		return id
	}
	if id := s.ipToID[canonicalIP(mgmtIP)]; id != "" {
		return id
	}

	generatedID := deriveRemoteDeviceID(hostname, chassisID, mgmtIP, fallbackID)
	if _, ok := s.devices[generatedID]; !ok {
		device := Device{
			ID:        generatedID,
			Hostname:  strings.TrimSpace(hostname),
			SysObject: "",
			ChassisID: strings.TrimSpace(chassisID),
		}
		if device.Hostname == "" {
			device.Hostname = generatedID
		}
		if ip := parseAddr(mgmtIP); ip.IsValid() {
			device.Addresses = []netip.Addr{ip}
		}
		s.devices[generatedID] = device
	}
	if host := canonicalHost(hostname); host != "" {
		if _, exists := s.hostToID[host]; !exists {
			s.hostToID[host] = generatedID
		}
	}
	if chassis := canonicalToken(chassisID); chassis != "" {
		if _, exists := s.chassisToID[chassis]; !exists {
			s.chassisToID[chassis] = generatedID
		}
	}
	if ip := canonicalIP(mgmtIP); ip != "" {
		if _, exists := s.ipToID[ip]; !exists {
			s.ipToID[ip] = generatedID
		}
	}
	return generatedID
}
