// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"strings"

	"github.com/gosnmp/gosnmp"
)

type topologySNMPRecHandler struct {
	gosnmp.Handler
	entries           []gosnmp.SnmpPDU
	byOID             map[string]gosnmp.SnmpPDU
	hiddenOIDPrefixes []string
	walkRoots         []string
}

func newTopologySNMPHandler(entries []gosnmp.SnmpPDU) *topologySNMPRecHandler {
	handler := &topologySNMPRecHandler{
		Handler: gosnmp.NewHandler(),
		entries: entries,
		byOID:   make(map[string]gosnmp.SnmpPDU, len(entries)),
	}
	for _, pdu := range entries {
		handler.byOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}
	return handler
}

func (h *topologySNMPRecHandler) addEntries(entries ...gosnmp.SnmpPDU) {
	for _, pdu := range entries {
		h.entries = append(h.entries, pdu)
		h.byOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}
}

func (h *topologySNMPRecHandler) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	variables := make([]gosnmp.SnmpPDU, 0, len(oids))
	for _, oid := range oids {
		key := strings.TrimPrefix(strings.TrimSpace(oid), ".")
		if h.isHiddenOID(key) {
			variables = append(variables, gosnmp.SnmpPDU{Name: key, Type: gosnmp.NoSuchObject})
			continue
		}
		if pdu, ok := h.byOID[key]; ok {
			variables = append(variables, pdu)
			continue
		}
		variables = append(variables, gosnmp.SnmpPDU{Name: key, Type: gosnmp.NoSuchObject})
	}
	return &gosnmp.SnmpPacket{Variables: variables}, nil
}

func (h *topologySNMPRecHandler) WalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	return h.walkAll(root), nil
}

func (h *topologySNMPRecHandler) BulkWalkAll(root string) ([]gosnmp.SnmpPDU, error) {
	return h.walkAll(root), nil
}

func (h *topologySNMPRecHandler) walkAll(root string) []gosnmp.SnmpPDU {
	root = strings.TrimPrefix(strings.TrimSpace(root), ".")
	h.walkRoots = append(h.walkRoots, root)
	prefix := root + "."
	var out []gosnmp.SnmpPDU
	for _, pdu := range h.entries {
		name := strings.TrimPrefix(pdu.Name, ".")
		if h.isHiddenOID(name) {
			continue
		}
		if name == root || strings.HasPrefix(name, prefix) {
			out = append(out, pdu)
		}
	}
	return out
}

func (h *topologySNMPRecHandler) hideOIDPrefix(prefix string) {
	prefix = strings.TrimPrefix(strings.TrimSpace(prefix), ".")
	if prefix != "" {
		h.hiddenOIDPrefixes = append(h.hiddenOIDPrefixes, prefix)
	}
}

func (h *topologySNMPRecHandler) isHiddenOID(oid string) bool {
	oid = strings.TrimPrefix(strings.TrimSpace(oid), ".")
	for _, prefix := range h.hiddenOIDPrefixes {
		if oid == prefix || strings.HasPrefix(oid, prefix+".") {
			return true
		}
	}
	return false
}

func (h *topologySNMPRecHandler) Version() gosnmp.SnmpVersion { return gosnmp.Version2c }
func (h *topologySNMPRecHandler) MaxOids() int                { return 60 }
