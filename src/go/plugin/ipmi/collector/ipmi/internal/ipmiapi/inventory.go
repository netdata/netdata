// SPDX-License-Identifier: GPL-3.0-or-later

package ipmiapi

import (
	"context"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

// SDRs are static between repository changes. Some BMCs return sentinel dates;
// the periodic refresh also catches metadata changes when timestamps cannot.
const inventoryRefresh = 5 * time.Minute

func sameInventory(a, b storage.GetSDRRepoInfoResponse) bool {
	return a.RecordCount == b.RecordCount && a.MostRecentAdditionTime.Equal(b.MostRecentAdditionTime) && a.MostRecentEraseTime.Equal(b.MostRecentEraseTime)
}

func (r *Reader) refresh(ctx context.Context) error {
	var info storage.GetSDRRepoInfoResponse
	if err := r.exchange(ctx, &storage.GetSDRRepoInfoRequest{}, &info); err != nil {
		return err
	}
	if !r.inventoryAt.IsZero() && sameInventory(info, r.inventoryInfo) && r.now().Sub(r.inventoryAt) < inventoryRefresh {
		return nil
	}
	var inventory []descriptor
	if info.RecordCount > 0 {
		seen := make(map[uint16]bool)
		id := uint16(0)
		for {
			if seen[id] {
				return fmt.Errorf("SDR chain repeats record %#04x", id)
			}
			seen[id] = true
			res, err := r.readSDR(ctx, id, info.SDROperationSupport.SupportReserveSDRRepo)
			if err != nil {
				return err
			}
			header, err := types.ParseSDRHeader(res.RecordData)
			if err != nil {
				return err
			}
			if len(res.RecordData) != int(header.RecordLength)+types.SDRRecordHeaderSize {
				return fmt.Errorf("SDR %#04x length does not match header", id)
			}
			if header.RecordType == types.SDRRecordTypeFullSensor || header.RecordType == types.SDRRecordTypeCompactSensor {
				sdr, err := types.ParseSDR(res.RecordData, res.NextRecordID)
				if err != nil {
					return err
				}
				inventory = append(inventory, describe(sdr)...)
			}
			if res.NextRecordID == 0xffff {
				break
			}
			id = res.NextRecordID
		}
	}
	// Do not publish a mixture from two repository generations.
	var after storage.GetSDRRepoInfoResponse
	if err := r.exchange(ctx, &storage.GetSDRRepoInfoRequest{}, &after); err != nil {
		return err
	}
	if !sameInventory(info, after) {
		return fmt.Errorf("SDR repository changed during discovery")
	}
	r.inventory = inventory
	r.inventoryInfo = after
	r.inventoryAt = r.now()
	return nil
}

func (r *Reader) readSDR(ctx context.Context, id uint16, reservable bool) (*storage.GetSDRResponse, error) {
	var res storage.GetSDRResponse
	err := r.exchange(ctx, &storage.GetSDRRequest{RecordID: id, ReadBytes: 0xff}, &res)
	if err == nil {
		return &res, nil
	}
	cc, ok := types.IsResponseError(err)
	if !ok || cc.CompletionCode() != types.CodeCannotReturnRequestedDataBytes {
		return nil, err
	}
	var reservation uint16
	if reservable {
		var reserved storage.ReserveSDRRepoResponse
		if err := r.exchange(ctx, &storage.ReserveSDRRepoRequest{}, &reserved); err != nil {
			return nil, err
		}
		reservation = reserved.ReservationID
	}
	var data []byte
	total := types.SDRRecordHeaderSize
	var next uint16
	for len(data) < total {
		offset := len(data)
		count := min(16, total-offset)
		// The one-byte offset can reach 255, while header+body can reach 260.
		if offset < 255 && offset+count > 255 {
			count = 255 - offset
		}
		var part storage.GetSDRResponse
		if err := r.exchange(ctx, &storage.GetSDRRequest{ReservationID: reservation, RecordID: id, ReadOffset: uint8(offset), ReadBytes: uint8(count)}, &part); err != nil {
			return nil, err
		}
		if len(part.RecordData) != count {
			return nil, fmt.Errorf("short partial SDR %#04x: got %d bytes, wanted %d", id, len(part.RecordData), count)
		}
		if offset > 0 && part.NextRecordID != next {
			return nil, fmt.Errorf("SDR chain changed during partial read")
		}
		next = part.NextRecordID
		data = append(data, part.RecordData...)
		if offset == 0 {
			total = types.SDRRecordHeaderSize + int(data[4])
		}
	}
	return &storage.GetSDRResponse{NextRecordID: next, RecordData: data}, nil
}
