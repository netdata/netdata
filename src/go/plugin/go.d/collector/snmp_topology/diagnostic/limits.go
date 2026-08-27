// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
	"math"
)

// ReaderLimits is caller policy, not part of diagnostic v1 validity. Every
// field is required so a reader cannot accidentally run without a bound.
type ReaderLimits struct {
	MaxStoredBytes    uint64
	MaxLogicalBytes   uint64
	MaxMemberBytes    uint64
	MaxMembers        uint64
	MaxDevices        uint64
	MaxProfiles       uint64
	MaxRows           uint64
	MaxTags           uint64
	MaxStringBytes    uint64
	MaxDNSRecords     uint64
	MaxOUIRecords     uint64
	MaxReferenceEdges uint64
	MaxNestingDepth   uint64
	MaxJSONTokens     uint64
	MaxReplayWork     uint64
}

func (l ReaderLimits) Validate() error {
	values := []struct {
		name  string
		value uint64
	}{
		{"MaxStoredBytes", l.MaxStoredBytes},
		{"MaxLogicalBytes", l.MaxLogicalBytes},
		{"MaxMemberBytes", l.MaxMemberBytes},
		{"MaxMembers", l.MaxMembers},
		{"MaxDevices", l.MaxDevices},
		{"MaxProfiles", l.MaxProfiles},
		{"MaxRows", l.MaxRows},
		{"MaxTags", l.MaxTags},
		{"MaxStringBytes", l.MaxStringBytes},
		{"MaxDNSRecords", l.MaxDNSRecords},
		{"MaxOUIRecords", l.MaxOUIRecords},
		{"MaxReferenceEdges", l.MaxReferenceEdges},
		{"MaxNestingDepth", l.MaxNestingDepth},
		{"MaxJSONTokens", l.MaxJSONTokens},
		{"MaxReplayWork", l.MaxReplayWork},
	}
	for _, item := range values {
		if item.value == 0 {
			return fmt.Errorf("%s must be nonzero", item.name)
		}
	}
	if l.MaxMemberBytes > l.MaxLogicalBytes {
		return errors.New("MaxMemberBytes must not exceed MaxLogicalBytes")
	}
	if l.MaxLogicalBytes > uint64(math.MaxInt64) || l.MaxMemberBytes > uint64(math.MaxInt64) {
		return errors.New("byte limits exceed supported reader size")
	}
	return nil
}

type budget struct {
	limits ReaderLimits

	logicalBytes   uint64
	members        uint64
	referenceEdges uint64
}

func newBudget(limits ReaderLimits) (*budget, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &budget{limits: limits}, nil
}

func (b *budget) addMember(ref ContentRef) error {
	if ref.LogicalLength > b.limits.MaxMemberBytes {
		return fmt.Errorf("member logical length %d exceeds limit %d", ref.LogicalLength, b.limits.MaxMemberBytes)
	}
	var err error
	if b.members, err = checkedAdd(b.members, 1); err != nil {
		return err
	}
	if b.members > b.limits.MaxMembers {
		return fmt.Errorf("member count %d exceeds limit %d", b.members, b.limits.MaxMembers)
	}
	if b.logicalBytes, err = checkedAdd(b.logicalBytes, ref.LogicalLength); err != nil {
		return err
	}
	if b.logicalBytes > b.limits.MaxLogicalBytes {
		return fmt.Errorf("logical bytes %d exceeds limit %d", b.logicalBytes, b.limits.MaxLogicalBytes)
	}
	return nil
}

func (b *budget) addReferences(count uint64) error {
	var err error
	if b.referenceEdges, err = checkedAdd(b.referenceEdges, count); err != nil {
		return err
	}
	if b.referenceEdges > b.limits.MaxReferenceEdges {
		return fmt.Errorf("reference edges %d exceeds limit %d", b.referenceEdges, b.limits.MaxReferenceEdges)
	}
	return nil
}

func checkedAdd(a, b uint64) (uint64, error) {
	if math.MaxUint64-a < b {
		return 0, errors.New("diagnostic budget arithmetic overflow")
	}
	return a + b, nil
}
