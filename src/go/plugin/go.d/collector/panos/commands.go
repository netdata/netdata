// SPDX-License-Identifier: GPL-3.0-or-later

package panos

const (
	systemInfoCommand  = "<show><system><info></info></system></show>"
	haStateCommand     = "<show><high-availability><state></state></high-availability></show>"
	environmentCommand = "<show><system><environmentals></environmentals></system></show>"
	licenseInfoCommand = "<request><license><info></info></license></request>"
	ipsecSACommand     = "<show><vpn><ipsec-sa></ipsec-sa></vpn></show>"
)
