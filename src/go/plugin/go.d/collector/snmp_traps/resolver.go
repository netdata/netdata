// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const maxMessageLen = 512

// renderMessage renders a trap description template into a human-readable MESSAGE.
// It resolves {varname}, {varname.raw}, {numeric.oid}, and special vars against the
// current entry's varbinds and profile definition.
func renderMessage(entry *TrapEntry, td *TrapDef) string {
	tmpl := td.Description
	if tmpl == "" {
		tmpl = "{TRAP_NAME} on {_HOSTNAME}."
	}

	result := renderTemplate(tmpl, entry, td)
	if len(result) > maxMessageLen {
		result = truncateUTF8(result, maxMessageLen-3) + "..."
	}
	return result
}

// renderLabels renders label templates from the profile into the entry's Labels map.
// Labels render as TRAP_TAG_<KEY_UPPERCASE> in later journal serialization.
func renderLabels(entry *TrapEntry, td *TrapDef) map[string]string {
	if len(td.Labels) == 0 {
		return nil
	}
	labels := make(map[string]string, len(td.Labels))
	for key, tmpl := range td.Labels {
		val := renderTemplate(tmpl, entry, td)
		labels[key] = val
	}
	return labels
}

// renderTemplate substitutes {var} references in a template string.
func renderTemplate(tmpl string, entry *TrapEntry, td *TrapDef) string {
	var buf strings.Builder
	buf.Grow(len(tmpl) + 128)

	i := 0
	for i < len(tmpl) {
		if tmpl[i] == '{' {
			end := strings.IndexByte(tmpl[i:], '}')
			if end == -1 {
				buf.WriteByte(tmpl[i])
				i++
				continue
			}
			ref := tmpl[i+1 : i+end]
			val := resolveReference(ref, entry, td)
			buf.WriteString(val)
			i += end + 1
		} else {
			buf.WriteByte(tmpl[i])
			i++
		}
	}
	return buf.String()
}

// resolveReference resolves a {var}, {var.raw}, or {special} reference.
func resolveReference(ref string, entry *TrapEntry, td *TrapDef) string {
	if ref == "" {
		return "<missing>"
	}

	// special vars
	if knownSpecialVars[ref] {
		return resolveSpecialVar(ref, entry)
	}

	// .raw suffix for raw numeric values
	if raw, ok := strings.CutSuffix(ref, ".raw"); ok {
		return resolveVarbindRaw(raw, entry, td)
	}

	// numeric OID fallback
	if isNumericOID(ref) {
		return resolveVarbindByOID(ref, entry, td)
	}

	// varbind name
	return resolveVarbindByName(ref, entry, td)
}

func resolveSpecialVar(ref string, entry *TrapEntry) string {
	switch ref {
	case "_HOSTNAME":
		if entry.DeviceHostname != "" {
			return entry.DeviceHostname
		}
		return entry.SourceIP
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

func resolveVarbindByName(name string, entry *TrapEntry, td *TrapDef) string {
	// Check profile varbinds by name
	if td != nil {
		if vb := td.varbindByName(name); vb != nil {
			return resolveVarbindValue(name, vb.OID, vb, entry)
		}
	}
	// Check raw varbinds from PDU
	return resolveRawVarbindByName(name, entry)
}

func resolveVarbindByOID(oid string, entry *TrapEntry, td *TrapDef) string {
	if td != nil {
		if vb := td.varbindByOID(oid); vb != nil {
			return resolveVarbindValue(vb.rawName, oid, vb, entry)
		}
	}
	return resolveRawVarbindByOID(oid, entry)
}

func resolveVarbindRaw(name string, entry *TrapEntry, td *TrapDef) string {
	oid := ""
	if td != nil {
		if vb := td.varbindByName(name); vb != nil {
			oid = vb.OID
		}
	}
	if oid == "" && isNumericOID(name) {
		oid = name
	}
	if oid == "" {
		return fmt.Sprintf("<unresolved:%s>", name)
	}
	return resolveRawVarbindByOID(oid, entry)
}

func resolveVarbindValue(name, oid string, vb *VarbindDef, entry *TrapEntry) string {
	for _, v := range entry.Varbinds {
		if v.OID == oid {
			return varbindDisplayValue(v, vb)
		}
	}
	return "<missing>"
}

func resolveRawVarbindByName(name string, entry *TrapEntry) string {
	for _, v := range entry.Varbinds {
		if v.Name == name {
			return varbindRawValue(v)
		}
	}
	return fmt.Sprintf("<unresolved:%s>", name)
}

func resolveRawVarbindByOID(oid string, entry *TrapEntry) string {
	for _, v := range entry.Varbinds {
		if v.OID == oid {
			return varbindRawValue(v)
		}
	}
	return "<missing>"
}

// varbindDisplayValue renders a varbind as a string, using enum labels when available.
func varbindDisplayValue(v VarbindValue, vb *VarbindDef) string {
	if vb != nil && len(vb.Enum) > 0 {
		key := fmt.Sprintf("%v", v.Value)
		if label, ok := vb.Enum[key]; ok {
			return label
		}
	}
	return varbindRawValue(v)
}

// varbindRawValue renders a varbind value as a plain string.
func varbindRawValue(v VarbindValue) string {
	switch val := v.Value.(type) {
	case nil:
		return ""
	case string:
		return val
	case []byte:
		return string(val)
	case int64:
		return fmt.Sprintf("%d", val)
	case uint64:
		return fmt.Sprintf("%d", val)
	case float64:
		return fmt.Sprintf("%v", val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", val)
	}
}

func truncateUTF8(s string, maxBytes int) string {
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
func resolve2TierVarbind(oid string, raw VarbindValue, td *TrapDef) VarbindValue {
	if td != nil {
		if vb := td.varbindByOID(oid); vb != nil {
			return VarbindValue{
				Name:  vb.rawName,
				OID:   oid,
				Type:  ASN1Type(vb.Type),
				Value: raw.Value,
				Enum:  resolveEnum(vb, raw.Value),
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
func resolveEnum(vb *VarbindDef, val any) string {
	if vb == nil || len(vb.Enum) == 0 {
		return ""
	}
	key := fmt.Sprintf("%v", val)
	if label, ok := vb.Enum[key]; ok {
		return label
	}
	return ""
}
