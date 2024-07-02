// SPDX-License-Identifier: GPL-3.0-or-later

package client

type LoadStats struct {
	NumOfClients int64
	BytesIn      int64
	BytesOut     int64
}

type Version struct {
	Major      int64
	Minor      int64
	Patch      int64
	Management int64
}

type Users []User

type User struct {
	CommonName     string
	RealAddress    string
	VirtualAddress string
	BytesReceived  int64
	BytesSent      int64
	ConnectedSince int64
	Username       string
}
