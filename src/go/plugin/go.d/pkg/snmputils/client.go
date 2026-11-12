// SPDX-License-Identifier: GPL-3.0-or-later

package snmputils

import (
	"github.com/gosnmp/gosnmp"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=client.go -destination=snmpmock/client.go -package=snmpmock

// ScalarClient is sufficient for system and device metadata acquisition.
type ScalarClient interface {
	Get(oids []string) (*gosnmp.SnmpPacket, error)
}

// Client is the request interface of Netdata's SNMP polling. The SNMP version,
// the maximum OIDs per request and max-repetitions are configuration values the
// caller passes; nothing reads them back from a client.
type Client interface {
	ScalarClient
	WalkAll(rootOid string) ([]gosnmp.SnmpPDU, error)
	BulkWalkAll(rootOid string, maxRepetitions uint32) ([]gosnmp.SnmpPDU, error)
}

// Session is a Client together with the lifecycle its owner drives.
type Session interface {
	Client
	Connect() error
	Close() error
}

// NewSession returns a configured handler as a Session. Each bulk walk sets the
// handler's max-repetitions to the value it is given.
func NewSession(handler gosnmp.Handler) Session {
	return handlerSession{handler: handler}
}

type handlerSession struct {
	handler gosnmp.Handler
}

func (s handlerSession) Connect() error { return s.handler.Connect() }

func (s handlerSession) Close() error { return s.handler.Close() }

func (s handlerSession) Get(oids []string) (*gosnmp.SnmpPacket, error) { return s.handler.Get(oids) }

func (s handlerSession) WalkAll(rootOid string) ([]gosnmp.SnmpPDU, error) {
	return s.handler.WalkAll(rootOid)
}

func (s handlerSession) BulkWalkAll(rootOid string, maxRepetitions uint32) ([]gosnmp.SnmpPDU, error) {
	s.handler.SetMaxRepetitions(maxRepetitions)
	return s.handler.BulkWalkAll(rootOid)
}

// MaxOIDsOrDefault is the maximum OIDs per request a client configured with
// maxOIDs uses: 0 takes the library default, as Connect does.
func MaxOIDsOrDefault(maxOIDs int) int {
	if maxOIDs == 0 {
		return gosnmp.MaxOids
	}
	return maxOIDs
}
