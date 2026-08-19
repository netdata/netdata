// SPDX-License-Identifier: GPL-3.0-or-later

package profilemetrics

import (
	"fmt"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/catalog"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func profileMetricPredicatesMatch(preds []profileMetricPredicate, entry *model.TrapEntry, td *catalog.TrapDef) bool {
	for _, pred := range preds {
		if !profileMetricPredicateMatches(pred, entry, td) {
			return false
		}
	}
	return true
}

func profileMetricPredicateMatches(pred profileMetricPredicate, entry *model.TrapEntry, td *catalog.TrapDef) bool {
	present, value, vb := profileMetricPredicateValue(pred, entry, td)
	result := profileMetricPredicateResult(pred, present, value, vb)
	if pred.Not && present {
		return !result
	}
	return result
}

func profileMetricPredicateValue(pred profileMetricPredicate, entry *model.TrapEntry, td *catalog.TrapDef) (bool, model.VarbindValue, *catalog.VarbindDef) {
	if pred.Field != "" {
		return profileMetricSyntheticFieldValue(pred.Field, entry)
	}
	vb := trapMetricVarbindByName(td, pred.Varbind)
	if vb == nil {
		return false, model.VarbindValue{}, nil
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	return ok, v, vb
}

func profileMetricSyntheticFieldValue(field string, entry *model.TrapEntry) (bool, model.VarbindValue, *catalog.VarbindDef) {
	var value string
	switch field {
	case "category":
		value = string(entry.Category)
	case "severity":
		value = string(entry.Severity)
	case "trap_name":
		value = entry.TrapName
	case "trap_oid":
		value = entry.TrapOID
	default:
		return false, model.VarbindValue{}, nil
	}
	if value == "" {
		return false, model.VarbindValue{}, nil
	}
	return true, model.VarbindValue{Value: value}, nil
}

func profileMetricPredicateResult(pred profileMetricPredicate, present bool, value model.VarbindValue, vb *catalog.VarbindDef) bool {
	if pred.Absent != nil {
		return *pred.Absent == !present
	}
	if pred.Exists != nil {
		return *pred.Exists == present
	}
	if !present {
		return false
	}
	if pred.Equals != nil {
		return profileMetricValueEquals(value, vb, pred.Equals)
	}
	if len(pred.In) > 0 {
		return slices.ContainsFunc(pred.In, func(want any) bool {
			return profileMetricValueEquals(value, vb, want)
		})
	}
	if pred.GreaterThan != nil || pred.LessThan != nil || len(pred.Range) > 0 {
		actual, ok := catalog.ParseMetricFloat(value.Value)
		if !ok {
			return false
		}
		if pred.GreaterThan != nil {
			want, ok := catalog.ParseMetricFloat(pred.GreaterThan)
			if !ok || actual <= want {
				return false
			}
		}
		if pred.LessThan != nil {
			want, ok := catalog.ParseMetricFloat(pred.LessThan)
			if !ok || actual >= want {
				return false
			}
		}
		if len(pred.Range) > 0 {
			low, okLow := catalog.ParseMetricFloat(pred.Range[0])
			high, okHigh := catalog.ParseMetricFloat(pred.Range[1])
			if !okLow || !okHigh || actual < low || actual > high {
				return false
			}
		}
		return true
	}
	return false
}

func profileMetricValueEquals(value model.VarbindValue, vb *catalog.VarbindDef, want any) bool {
	actual := model.VarbindRawValue(value)
	if vb != nil && len(vb.Enum) > 0 {
		if label := catalog.ResolveEnum(vb, value.Value); label != "" && label == fmt.Sprintf("%v", want) {
			return true
		}
	}
	return actual == fmt.Sprintf("%v", want)
}

type profileMetricValueStatus int

const (
	profileMetricValueOK profileMetricValueStatus = iota
	profileMetricValueMissing
	profileMetricValueInvalid
)

func profileMetricNumericVarbindValue(entry *model.TrapEntry, vb *catalog.VarbindDef) (float64, profileMetricValueStatus) {
	if vb == nil {
		return 0, profileMetricValueInvalid
	}
	v, ok := model.FindVarbindForProfileOID(entry.Varbinds, vb.OID)
	if !ok {
		return 0, profileMetricValueMissing
	}
	value, ok := catalog.ParseMetricFloat(v.Value)
	if !ok {
		return 0, profileMetricValueInvalid
	}
	if strings.EqualFold(strings.TrimSpace(vb.Type), "timeticks") {
		value /= 100
	}
	return value, profileMetricValueOK
}
