// SPDX-License-Identifier: GPL-3.0-or-later

package cato_networks

const accountSnapshotDocument = `query accountSnapshot($siteIDs: [ID!], $accountID: ID) {
	accountSnapshot(accountID: $accountID) {
		sites(siteIDs: $siteIDs) {
			id
			connectivityStatus
			degradedStatus {
				isDegraded
			}
			operationalStatus
			popName
			hostCount
			info {
				name
				type
				description
				countryCode
				region
				countryName
				connType
			}
			devices {
				id
				name
				identifier
				connected
				haRole
				lastPopName
				internalIP
				socketInfo {
					id
					serial
					version
				}
				interfaces {
					connected
					id
					name
					popName
					tunnelUptime
					tunnelRemoteIP
					type
					info {
						id
						name
						upstreamBandwidth
						downstreamBandwidth
					}
				}
				interfacesLinkState {
					id
					up
				}
			}
		}
	}
}`

type accountSnapshot struct {
	Sites []*accountSnapshotSite `json:"sites"`
}

type accountSnapshotSite struct {
	ConnectivityStatus *string                        `json:"connectivityStatus"`
	DegradedStatus     *accountSnapshotDegradedStatus `json:"degradedStatus"`
	Devices            []*accountSnapshotDevice       `json:"devices"`
	HostCount          *int64                         `json:"hostCount"`
	ID                 *string                        `json:"id"`
	Info               *accountSnapshotSiteInfo       `json:"info"`
	OperationalStatus  *string                        `json:"operationalStatus"`
	PopName            *string                        `json:"popName"`
}

type accountSnapshotDegradedStatus struct {
	IsDegraded bool `json:"isDegraded"`
}

type accountSnapshotSiteInfo struct {
	ConnType    *string `json:"connType"`
	CountryCode *string `json:"countryCode"`
	CountryName *string `json:"countryName"`
	Description *string `json:"description"`
	Name        *string `json:"name"`
	Region      *string `json:"region"`
	Type        *string `json:"type"`
}

type accountSnapshotDevice struct {
	Connected           *bool                                `json:"connected"`
	HaRole              *string                              `json:"haRole"`
	ID                  *string                              `json:"id"`
	Identifier          *string                              `json:"identifier"`
	Interfaces          []*accountSnapshotInterface          `json:"interfaces"`
	InterfacesLinkState []*accountSnapshotInterfaceLinkState `json:"interfacesLinkState"`
	InternalIP          *string                              `json:"internalIP"`
	LastPopName         *string                              `json:"lastPopName"`
	Name                *string                              `json:"name"`
	SocketInfo          *accountSnapshotSocketInfo           `json:"socketInfo"`
}

type accountSnapshotSocketInfo struct {
	ID      *string `json:"id"`
	Serial  *string `json:"serial"`
	Version *string `json:"version"`
}

type accountSnapshotInterface struct {
	Connected      *bool                         `json:"connected"`
	ID             *string                       `json:"id"`
	Info           *accountSnapshotInterfaceInfo `json:"info"`
	Name           *string                       `json:"name"`
	PopName        *string                       `json:"popName"`
	TunnelRemoteIP *string                       `json:"tunnelRemoteIP"`
	TunnelUptime   *int64                        `json:"tunnelUptime"`
	Type           *string                       `json:"type"`
}

type accountSnapshotInterfaceInfo struct {
	DownstreamBandwidth *int64  `json:"downstreamBandwidth"`
	ID                  string  `json:"id"`
	Name                *string `json:"name"`
	UpstreamBandwidth   *int64  `json:"upstreamBandwidth"`
}

type accountSnapshotInterfaceLinkState struct {
	ID *string `json:"id"`
	Up *bool   `json:"up"`
}
