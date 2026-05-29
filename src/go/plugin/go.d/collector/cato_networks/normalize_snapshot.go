// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

import (
	"fmt"

	catosdk "github.com/catonetworks/cato-go-sdk"
)

func normalizeSnapshot(snapshot *catosdk.AccountSnapshot, siteNames map[string]string) (map[string]*siteState, []string) {
	out := make(map[string]*siteState)
	var order []string

	for _, raw := range snapshot.GetAccountSnapshot().GetSites() {
		siteID := derefZero(raw.GetID())
		if siteID == "" {
			continue
		}

		var (
			infoName    string
			description string
			countryCode string
			countryName string
			region      string
			siteType    string
			connType    string
		)
		if info := raw.GetInfoSiteSnapshot(); info != nil {
			infoName = derefZero(info.GetName())
			description = derefZero(info.GetDescription())
			countryCode = derefZero(info.GetCountryCode())
			countryName = derefZero(info.GetCountryName())
			region = derefZero(info.GetRegion())
			if info.GetType() != nil {
				siteType = fmt.Sprint(*info.GetType())
			}
			if info.GetConnType() != nil {
				connType = fmt.Sprint(*info.GetConnType())
			}
		}
		site := &siteState{
			ID:                 siteID,
			Name:               siteDisplayName(siteID, siteNames, infoName, ""),
			Description:        description,
			ConnectivityStatus: normalizeStatus(connectivityStatusString(raw.GetConnectivityStatusSiteSnapshot())),
			OperationalStatus:  normalizeStatus(operationalStatusString(raw.GetOperationalStatusSiteSnapshot())),
			PopName:            derefZero(raw.GetPopName()),
			CountryCode:        countryCode,
			CountryName:        countryName,
			Region:             region,
			SiteType:           siteType,
			ConnectionType:     connType,
			LastConnected:      derefZero(raw.GetLastConnected()),
			ConnectedSince:     derefZero(raw.GetConnectedSince()),
			HostCount:          derefZero(raw.GetHostCount()),
			Interfaces:         make(map[string]*interfaceState),
		}

		for _, dev := range raw.GetDevices() {
			device := deviceState{
				ID:             derefZero(dev.GetID()),
				Name:           derefZero(dev.GetName()),
				Type:           derefZero(dev.GetType()),
				Connected:      derefZero(dev.GetConnected()),
				HaRole:         derefZero(dev.GetHaRole()),
				InternalIP:     derefZero(dev.GetInternalIP()),
				LastPopName:    derefZero(dev.GetLastPopName()),
				ConnectedSince: derefZero(dev.GetConnectedSince()),
			}
			if socket := dev.GetSocketInfo(); socket != nil {
				device.SocketID = derefZero(socket.GetID())
				device.SocketSerial = derefZero(socket.GetSerial())
				device.SocketVersion = derefZero(socket.GetVersion())
			}
			site.Devices = append(site.Devices, device)

			linkStateByID := make(map[string]*catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_InterfacesLinkState)
			for _, linkState := range dev.GetInterfacesLinkState() {
				if id := derefZero(linkState.GetID()); id != "" {
					linkStateByID[id] = linkState
				}
			}
			for _, rawIface := range dev.GetInterfaces() {
				iface := normalizeSnapshotInterface(rawIface)
				if iface.ID == "" && iface.Name == "" {
					continue
				}
				if linkState := linkStateByID[iface.ID]; linkState != nil {
					iface.LinkUp = derefZero(linkState.GetUp())
				}
				key := interfaceKey(iface.ID, iface.Name)
				site.Interfaces[key] = &iface
			}
		}

		out[siteID] = site
		order = append(order, siteID)
	}

	return out, order
}

func normalizeSnapshotInterface(raw *catosdk.AccountSnapshot_AccountSnapshot_Sites_Devices_Interfaces) interfaceState {
	iface := interfaceState{
		ID:             derefZero(raw.GetID()),
		Name:           derefZero(raw.GetName()),
		Type:           derefZero(raw.GetType()),
		Connected:      derefZero(raw.GetConnected()),
		PopName:        derefZero(raw.GetPopName()),
		TunnelRemoteIP: derefZero(raw.GetTunnelRemoteIP()),
		TunnelUptime:   derefZero(raw.GetTunnelUptime()),
		PhysicalPort:   derefZero(raw.GetPhysicalPort()),
	}
	if info := raw.GetInfoInterfaceSnapshot(); info != nil {
		if iface.ID == "" {
			iface.ID = info.GetID()
		}
		if iface.Name == "" {
			iface.Name = derefZero(info.GetName())
		}
		iface.DestType = derefZero(info.GetDestType())
		iface.UpstreamBandwidth = derefZero(info.GetUpstreamBandwidth())
		iface.DownstreamBandwidth = derefZero(info.GetDownstreamBandwidth())
	}
	return iface
}
