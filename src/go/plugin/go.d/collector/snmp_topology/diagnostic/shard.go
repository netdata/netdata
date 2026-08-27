// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostic

import (
	"errors"
	"fmt"
)

// ShardGeometryV1 identifies one deterministic half-open range within an
// ordered diagnostic section. Coordinates are semantic; manifest member order
// is only an inventory order.
type ShardGeometryV1 struct {
	CaptureID    uint64 `json:"capture_id"`
	Registration uint64 `json:"registration"`
	Section      string `json:"section"`
	Phase        uint32 `json:"phase"`
	Context      uint32 `json:"context"`
	Profile      uint32 `json:"profile"`
	Shard        uint32 `json:"shard"`
	ShardCount   uint32 `json:"shard_count"`
	FirstOrdinal uint64 `json:"first_ordinal"`
	RecordCount  uint64 `json:"record_count"`
}

func (g ShardGeometryV1) Validate() error {
	if g.CaptureID == 0 {
		return errors.New("capture id must be nonzero")
	}
	if g.Registration == 0 {
		return errors.New("registration must be nonzero")
	}
	if err := validateID("section", g.Section); err != nil {
		return err
	}
	if g.Phase == 0 {
		return errors.New("phase must be nonzero")
	}
	if g.ShardCount == 0 {
		return errors.New("shard count must be nonzero")
	}
	if g.Shard >= g.ShardCount {
		return fmt.Errorf("shard index %d is outside shard count %d", g.Shard, g.ShardCount)
	}
	if g.RecordCount == 0 {
		return errors.New("record count must be nonzero")
	}
	if _, err := checkedAdd(g.FirstOrdinal, g.RecordCount); err != nil {
		return fmt.Errorf("ordinal range: %w", err)
	}
	return nil
}

func ValidateShardSequence(shards []ShardGeometryV1, expectedRecords uint64) error {
	if expectedRecords == 0 {
		if len(shards) != 0 {
			return errors.New("zero-record section contains shards")
		}
		return nil
	}
	if len(shards) == 0 {
		return errors.New("nonempty section contains no shards")
	}

	first := shards[0]
	if first.FirstOrdinal != 0 {
		return fmt.Errorf("first shard starts at ordinal %d", first.FirstOrdinal)
	}
	if uint64(first.ShardCount) != uint64(len(shards)) {
		return fmt.Errorf("declared shard count %d does not match inventory %d", first.ShardCount, len(shards))
	}

	next := uint64(0)
	for i, shard := range shards {
		if err := shard.Validate(); err != nil {
			return fmt.Errorf("shards[%d]: %w", i, err)
		}
		if shard.CaptureID != first.CaptureID || shard.Registration != first.Registration ||
			shard.Section != first.Section || shard.Phase != first.Phase || shard.Context != first.Context ||
			shard.Profile != first.Profile {
			return fmt.Errorf("shards[%d] changes its owner or semantic coordinates", i)
		}
		if shard.ShardCount != first.ShardCount || shard.Shard != uint32(i) {
			return fmt.Errorf("shards[%d] has invalid shard coordinate %d/%d", i, shard.Shard, shard.ShardCount)
		}
		if shard.FirstOrdinal != next {
			return fmt.Errorf("shards[%d] starts at %d, expected %d", i, shard.FirstOrdinal, next)
		}
		var err error
		next, err = checkedAdd(next, shard.RecordCount)
		if err != nil {
			return err
		}
	}
	if next != expectedRecords {
		return fmt.Errorf("shards cover %d records, expected %d", next, expectedRecords)
	}
	return nil
}
