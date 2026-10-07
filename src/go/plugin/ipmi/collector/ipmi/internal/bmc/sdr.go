// SPDX-License-Identifier: GPL-3.0-or-later

package bmc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bougou/go-ipmi/pkg/command/storage"
	"github.com/bougou/go-ipmi/pkg/types"
)

const (
	// inventoryMaxAge forces rediscovery on BMCs whose repository timestamps do
	// not change with its content (sentinel or constant dates).
	inventoryMaxAge = 5 * time.Minute

	firstRecordID = 0x0000
	lastRecordID  = 0xffff

	// readEntireRecord asks Get SDR for the whole record in one response.
	readEntireRecord = 0xff
	// partialReadSize is the chunk size once the BMC refuses whole-record reads.
	partialReadSize = 16
	// maxReadOffset is the largest one-byte Get SDR offset. A record (five-byte
	// header plus up to 255 body bytes) can extend past it.
	maxReadOffset = 0xff
	// recordLengthOffset is the header byte holding the record body length.
	recordLengthOffset = 4
)

// refreshInventory rediscovers the SDR repository when it changed or the cached
// inventory is older than inventoryMaxAge.
func (r *Reader) refreshInventory(ctx context.Context) error {
	var repo storage.GetSDRRepoInfoResponse
	if err := r.exchange(ctx, &storage.GetSDRRepoInfoRequest{}, &repo); err != nil {
		return err
	}
	fresh := !r.inventoryReadAt.IsZero() && r.now().Sub(r.inventoryReadAt) < inventoryMaxAge
	if fresh && sameRepository(repo, r.inventoryRepo) {
		return nil
	}

	inventory, err := r.discoverSensors(ctx, repo)
	if err != nil {
		return err
	}

	// Do not publish a mixture of two repository generations.
	var after storage.GetSDRRepoInfoResponse
	if err := r.exchange(ctx, &storage.GetSDRRepoInfoRequest{}, &after); err != nil {
		return err
	}
	if !sameRepository(repo, after) {
		return errors.New("SDR repository changed during discovery")
	}

	r.inventory = inventory
	r.inventoryRepo = after
	r.inventoryReadAt = r.now()
	return nil
}

// discoverSensors walks the SDR chain and describes its full and compact sensor records.
func (r *Reader) discoverSensors(ctx context.Context, repo storage.GetSDRRepoInfoResponse) ([]descriptor, error) {
	// An empty repository has no first record to read.
	if repo.RecordCount == 0 {
		return nil, nil
	}

	var sensors []descriptor
	seen := make(map[uint16]bool)
	for id := uint16(firstRecordID); id != lastRecordID; {
		if seen[id] {
			return nil, fmt.Errorf("SDR chain repeats record %#04x", id)
		}
		seen[id] = true

		record, err := r.readRecord(ctx, id, repo.SDROperationSupport.SupportReserveSDRRepo)
		if err != nil {
			return nil, err
		}
		header, err := types.ParseSDRHeader(record.RecordData)
		if err != nil {
			return nil, err
		}
		if len(record.RecordData) != types.SDRRecordHeaderSize+int(header.RecordLength) {
			return nil, fmt.Errorf("SDR %#04x length does not match header", id)
		}
		if header.RecordType == types.SDRRecordTypeFullSensor || header.RecordType == types.SDRRecordTypeCompactSensor {
			sdr, err := types.ParseSDR(record.RecordData, record.NextRecordID)
			if err != nil {
				return nil, err
			}
			sensors = append(sensors, describeSDR(sdr)...)
		}
		id = record.NextRecordID
	}
	return sensors, nil
}

// readRecord reads one SDR, in parts when the BMC cannot return it in one response.
func (r *Reader) readRecord(ctx context.Context, id uint16, reservable bool) (*storage.GetSDRResponse, error) {
	req := &storage.GetSDRRequest{
		RecordID:  id,
		ReadBytes: readEntireRecord,
	}
	var res storage.GetSDRResponse
	err := r.exchange(ctx, req, &res)
	if err == nil {
		return &res, nil
	}
	if cc, ok := types.IsResponseError(err); !ok || cc.CompletionCode() != types.CodeCannotReturnRequestedDataBytes {
		return nil, err
	}
	return r.readRecordInParts(ctx, id, reservable)
}

// readRecordInParts reads the header first, then the body it announces, under a
// repository reservation when the BMC supports one.
func (r *Reader) readRecordInParts(ctx context.Context, id uint16, reservable bool) (*storage.GetSDRResponse, error) {
	var reservation uint16
	if reservable {
		var res storage.ReserveSDRRepoResponse
		if err := r.exchange(ctx, &storage.ReserveSDRRepoRequest{}, &res); err != nil {
			return nil, err
		}
		reservation = res.ReservationID
	}

	var data []byte
	var next uint16
	total := types.SDRRecordHeaderSize
	for len(data) < total {
		offset := len(data)
		count := min(partialReadSize, total-offset)
		// End a chunk at the last addressable offset so the next one can start there.
		if offset < maxReadOffset && offset+count > maxReadOffset {
			count = maxReadOffset - offset
		}

		req := &storage.GetSDRRequest{
			ReservationID: reservation,
			RecordID:      id,
			ReadOffset:    uint8(offset),
			ReadBytes:     uint8(count),
		}
		var part storage.GetSDRResponse
		if err := r.exchange(ctx, req, &part); err != nil {
			return nil, err
		}
		if len(part.RecordData) != count {
			return nil, fmt.Errorf("short partial SDR %#04x: got %d bytes, wanted %d", id, len(part.RecordData), count)
		}
		if offset > 0 && part.NextRecordID != next {
			return nil, errors.New("SDR chain changed during partial read")
		}

		next = part.NextRecordID
		data = append(data, part.RecordData...)
		if offset == 0 {
			total = types.SDRRecordHeaderSize + int(data[recordLengthOffset])
		}
	}
	return &storage.GetSDRResponse{
		NextRecordID: next,
		RecordData:   data,
	}, nil
}

// sameRepository compares the repository fields that change with its content.
func sameRepository(a, b storage.GetSDRRepoInfoResponse) bool {
	return a.RecordCount == b.RecordCount &&
		a.MostRecentAdditionTime.Equal(b.MostRecentAdditionTime) &&
		a.MostRecentEraseTime.Equal(b.MostRecentEraseTime)
}
