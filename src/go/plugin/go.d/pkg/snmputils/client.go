// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"context"

	"github.com/netdata/gosnmp"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=client.go -destination=snmpmock/client.go -package=snmpmock

// ScalarClient is sufficient for system and device metadata acquisition.
type ScalarClient interface {
	Get(ctx context.Context, oids []string) (*gosnmp.Message, error)
}

// Client is the request interface of Netdata's SNMP polling, which
// *gosnmp.Client satisfies. The SNMP version, the maximum OIDs per request and
// max-repetitions are configuration values the caller passes; nothing reads
// them back from a client.
type Client interface {
	ScalarClient
	WalkAll(ctx context.Context, rootOid string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error)
	BulkWalkAll(ctx context.Context, rootOid string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error)
}

// Session is a Client together with the lifecycle its owner drives.
type Session interface {
	Client
	Connect(ctx context.Context) error
	Close() error
}

// MaxOIDsOrDefault is the maximum OIDs per request a client configured with
// maxOIDs uses: 0 takes the library default, as gosnmp.NewClient does.
func MaxOIDsOrDefault(maxOIDs int) int {
	if maxOIDs == 0 {
		return gosnmp.MaxOids
	}
	return maxOIDs
}
