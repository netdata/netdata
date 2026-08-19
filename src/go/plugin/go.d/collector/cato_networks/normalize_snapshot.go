// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

func normalizeSnapshot(
	snapshot *accountSnapshot,
	siteNames map[string]string,
) (map[string]*siteState, []string) {
	if snapshot == nil {
		return map[string]*siteState{}, nil
	}

	out := make(map[string]*siteState, len(snapshot.Sites))
	order := make([]string, 0, len(snapshot.Sites))

	for _, raw := range snapshot.Sites {
		if raw == nil {
			continue
		}
		siteID := derefZero(raw.ID)
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
		if info := raw.Info; info != nil {
			infoName = derefZero(info.Name)
			description = derefZero(info.Description)
			countryCode = derefZero(info.CountryCode)
			countryName = derefZero(info.CountryName)
			region = derefZero(info.Region)
			siteType = derefZero(info.Type)
			connType = derefZero(info.ConnType)
		}
		site := &siteState{
			ID:          siteID,
			Name:        siteDisplayName(siteID, siteNames, infoName, ""),
			Description: description,
			ConnectivityStatus: normalizeSnapshotConnectivity(
				raw.ConnectivityStatus,
				raw.DegradedStatus,
			),
			OperationalStatus: normalizeStatus(derefZero(raw.OperationalStatus)),
			PopName:           derefZero(raw.PopName),
			CountryCode:       countryCode,
			CountryName:       countryName,
			Region:            region,
			SiteType:          siteType,
			ConnectionType:    connType,
			HostCount:         raw.HostCount,
			Interfaces:        make(map[string]*interfaceState),
		}

		for _, dev := range raw.Devices {
			if dev == nil {
				continue
			}
			device := deviceState{
				ID:          derefZero(dev.ID),
				Identifier:  derefZero(dev.Identifier),
				Name:        derefZero(dev.Name),
				Connected:   dev.Connected,
				HaRole:      derefZero(dev.HaRole),
				InternalIP:  derefZero(dev.InternalIP),
				LastPopName: derefZero(dev.LastPopName),
			}
			if socket := dev.SocketInfo; socket != nil {
				device.SocketID = derefZero(socket.ID)
				device.SocketSerial = derefZero(socket.Serial)
				device.SocketVersion = derefZero(socket.Version)
			}
			site.Devices = append(site.Devices, device)
			deviceID := stableDeviceID(device)
			deviceName := deviceDisplayName(device)

			linkStateByID := make(
				map[string]*accountSnapshotInterfaceLinkState,
				len(dev.InterfacesLinkState),
			)
			for _, linkState := range dev.InterfacesLinkState {
				if linkState == nil {
					continue
				}
				if id := derefZero(linkState.ID); id != "" {
					linkStateByID[id] = linkState
				}
			}
			for _, rawIface := range dev.Interfaces {
				if rawIface == nil {
					continue
				}
				iface := normalizeSnapshotInterface(rawIface)
				if iface.ID == "" && iface.Name == "" {
					continue
				}
				iface.DeviceID = deviceID
				iface.DeviceName = deviceName
				iface.DeviceSocketID = device.SocketID
				iface.DeviceSocketSerial = device.SocketSerial
				if linkState := linkStateByID[iface.ID]; linkState != nil {
					iface.LinkUp = linkState.Up
				}
				key := snapshotInterfaceKey(iface.DeviceID, iface.ID, iface.Name)
				site.Interfaces[key] = &iface
			}
		}

		out[siteID] = site
		order = append(order, siteID)
	}

	return out, order
}

func normalizeSnapshotConnectivity(status *string, degraded *accountSnapshotDegradedStatus) string {
	normalized := normalizeStatus(derefZero(status))
	switch normalized {
	case "connected":
		if degraded != nil && degraded.IsDegraded {
			return "degraded"
		}
		return "connected"
	case "disconnected":
		return "disconnected"
	default:
		return "unknown"
	}
}

func normalizeSnapshotInterface(raw *accountSnapshotInterface) interfaceState {
	iface := interfaceState{
		ID:             derefZero(raw.ID),
		Name:           derefZero(raw.Name),
		Type:           derefZero(raw.Type),
		Connected:      raw.Connected,
		PopName:        derefZero(raw.PopName),
		TunnelRemoteIP: derefZero(raw.TunnelRemoteIP),
		TunnelUptime:   raw.TunnelUptime,
	}
	if info := raw.Info; info != nil {
		if iface.ID == "" {
			iface.ID = info.ID
		}
		if iface.Name == "" {
			iface.Name = derefZero(info.Name)
		}
		iface.UpstreamBandwidth = derefZero(info.UpstreamBandwidth)
		iface.DownstreamBandwidth = derefZero(info.DownstreamBandwidth)
	}
	return iface
}
