// SPDX-License-Identifier: GPL-3.0-or-later

package catalog

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

const MaxMessageLen = 512

// renderMessage renders a trap description into a human-readable MESSAGE.
func RenderMessage(entry *TrapEntry, td *TrapDef) string {
	tmpl := ""
	if td != nil {
		tmpl = td.Description
	}

	var result string
	if tmpl == "" {
		result = resolveSpecialVar("TRAP_NAME", entry) + " on " + resolveSpecialVar("_HOSTNAME", entry) + "."
	} else if td.descriptionTemplate != nil {
		result = renderGoProfileTemplate(td.descriptionTemplate, entry, td)
	} else {
		result = tmpl
	}
	if len(result) > MaxMessageLen {
		result = TruncateUTF8(result, MaxMessageLen-3) + "..."
	}
	return result
}

// renderLabels renders label templates from the profile into the entry's Labels map.
// Labels render as TRAP_TAG_<KEY_UPPERCASE> in later journal serialization.
func RenderLabels(entry *TrapEntry, td *TrapDef) map[string]string {
	if len(td.Labels) == 0 {
		return nil
	}
	labels := make(map[string]string, len(td.Labels))
	for key, tmpl := range td.Labels {
		if td.labelTemplates != nil && td.labelTemplates[key] != nil {
			labels[key] = renderGoProfileTemplate(td.labelTemplates[key], entry, td)
			continue
		}
		labels[key] = tmpl
	}
	return labels
}

func EntryHasUnresolvedTemplate(entry *TrapEntry) bool {
	if entry == nil {
		return false
	}
	if hasUnresolvedTemplateMarker(entry.Message) {
		return true
	}
	for _, v := range entry.Labels {
		if hasUnresolvedTemplateMarker(v) {
			return true
		}
	}
	return false
}

func hasUnresolvedTemplateMarker(s string) bool {
	return strings.Contains(s, "<unresolved:") || strings.Contains(s, "<missing>")
}

func resolveSpecialVar(ref string, entry *TrapEntry) string {
	switch ref {
	case "_HOSTNAME":
		if entry.DeviceHostname != "" {
			return entry.DeviceHostname
		}
		if entry.SourceIP != "" {
			return entry.SourceIP
		}
		return entry.SourceUDPPeer
	case "TRAP_SOURCE_IP":
		return entry.SourceIP
	case "TRAP_NAME":
		return entry.TrapName
	case "TRAP_DEVICE_VENDOR":
		return entry.DeviceVendor
	case "TRAP_INTERFACE":
		return entry.TopologyInterface
	case "TRAP_NEIGHBORS":
		return entry.TopologyNeighbors
	default:
		return ""
	}
}

func FindVarbindDefForObservedOID(td *TrapDef, observedOID string) *VarbindDef {
	if td == nil || observedOID == "" || td.SharedVarbinds == nil {
		return nil
	}
	if vb := td.varbindByOID(observedOID); vb != nil {
		return vb
	}

	var best *VarbindDef
	bestLen := -1
	for oid, vb := range td.SharedVarbinds {
		if vb == nil {
			continue
		}
		profileOID := vb.OID
		if profileOID == "" {
			profileOID = oid
		}
		if model.OIDMatchesColumn(profileOID, observedOID) && len(profileOID) > bestLen {
			best = vb
			bestLen = len(profileOID)
		}
	}
	return best
}

// varbindDisplayValue renders a varbind as a string, using enum labels when available.
func varbindDisplayValue(v VarbindValue, vb *VarbindDef) string {
	if vb != nil && len(vb.Enum) > 0 {
		key := fmt.Sprintf("%v", v.Value)
		if label, ok := vb.Enum[key]; ok {
			return label
		}
	}
	return model.VarbindRawValue(v)
}

func TruncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		_, size := utf8.DecodeLastRuneInString(s[:maxBytes])
		if size == 0 {
			maxBytes--
		} else {
			maxBytes -= size
		}
	}
	return s[:maxBytes]
}

// resolve2TierVarbind is the 2-tier varbind resolution for assembling a TrapEntry.
// 1. Profile inline varbinds table (OID → VarbindDef with name, type, enum)
// 2. Raw fallback (OID-keyed, ASN.1-decoded type only)
func ResolveVarbind(oid string, raw VarbindValue, td *TrapDef) VarbindValue {
	if td != nil {
		if vb := FindVarbindDefForObservedOID(td, oid); vb != nil {
			return VarbindValue{
				Name:  vb.RawName,
				OID:   oid,
				Type:  ASN1Type(vb.Type),
				Value: raw.Value,
				Enum:  ResolveEnum(vb, raw.Value),
			}
		}
	}

	if raw.Name != "" {
		return raw
	}
	raw.Name = ""
	raw.Enum = ""
	return raw
}

// resolveEnum returns the enum label for a varbind value if applicable.
func ResolveEnum(vb *VarbindDef, val any) string {
	if vb == nil || len(vb.Enum) == 0 {
		return ""
	}
	key := fmt.Sprintf("%v", val)
	if label, ok := vb.Enum[key]; ok {
		return label
	}
	return ""
}
