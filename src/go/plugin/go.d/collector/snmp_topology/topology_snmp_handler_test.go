// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"strings"

	"github.com/netdata/gosnmp"
)

type topologySNMPRecHandler struct {
	entries           []gosnmp.VarBind
	byOID             map[string]gosnmp.VarBind
	hiddenOIDPrefixes []string
	walkRoots         []string
}

func newTopologySNMPHandler(entries []gosnmp.VarBind) *topologySNMPRecHandler {
	handler := &topologySNMPRecHandler{
		entries: entries,
		byOID:   make(map[string]gosnmp.VarBind, len(entries)),
	}
	for _, pdu := range entries {
		handler.byOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}
	return handler
}

func (h *topologySNMPRecHandler) addEntries(entries ...gosnmp.VarBind) {
	for _, pdu := range entries {
		h.entries = append(h.entries, pdu)
		h.byOID[strings.TrimPrefix(pdu.Name, ".")] = pdu
	}
}

func (h *topologySNMPRecHandler) Get(ctx context.Context, oids []string) (*gosnmp.Message, error) {
	variables := make([]gosnmp.VarBind, 0, len(oids))
	for _, oid := range oids {
		key := strings.TrimPrefix(strings.TrimSpace(oid), ".")
		if h.isHiddenOID(key) {
			variables = append(variables, gosnmp.VarBind{Name: key, Value: gosnmp.NoSuchObjectValue()})
			continue
		}
		if pdu, ok := h.byOID[key]; ok {
			variables = append(variables, pdu)
			continue
		}
		variables = append(variables, gosnmp.VarBind{Name: key, Value: gosnmp.NoSuchObjectValue()})
	}
	return &gosnmp.Message{VarBinds: variables}, nil
}

func (h *topologySNMPRecHandler) WalkAll(ctx context.Context, root string) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	return h.walkAll(root), gosnmp.WalkEnd{}, nil
}

func (h *topologySNMPRecHandler) BulkWalkAll(ctx context.Context, root string, maxRepetitions uint32) ([]gosnmp.VarBind, gosnmp.WalkEnd, error) {
	return h.walkAll(root), gosnmp.WalkEnd{}, nil
}

func (h *topologySNMPRecHandler) walkAll(root string) []gosnmp.VarBind {
	root = strings.TrimPrefix(strings.TrimSpace(root), ".")
	h.walkRoots = append(h.walkRoots, root)
	prefix := root + "."
	var out []gosnmp.VarBind
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
