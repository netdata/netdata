package protocol

import (
	"testing"
)

// ---------------------------------------------------------------------------
//  Fuzz targets for all decode paths.
//
//  Each target feeds arbitrary bytes to a decode function and, if the decode
//  succeeds, exercises the result. The invariant: no input may cause a panic.
// ---------------------------------------------------------------------------

func FuzzDecodeHeader(f *testing.F) {
	// Seed with a valid header.
	var seed [HeaderSize]byte
	h := Header{
		Magic: MagicMsg, Version: Version, HeaderLen: HeaderLen,
		Kind: KindRequest, Code: MethodIncrement, PayloadLen: 0,
		ItemCount: 1, MessageID: 1,
	}
	h.Encode(seed[:])
	f.Add(seed[:])

	// Seed with truncated and zeroed inputs.
	f.Add([]byte{})
	f.Add(make([]byte, 31))
	f.Add(make([]byte, 32))

	f.Fuzz(func(t *testing.T, data []byte) {
		hdr, err := DecodeHeader(data)
		if err != nil {
			return
		}
		// Exercise the decoded result.
		_ = hdr.Magic
		_ = hdr.Version
		_ = hdr.Kind
		_ = hdr.Flags
		_ = hdr.Code
		_ = hdr.TransportStatus
		_ = hdr.PayloadLen
		_ = hdr.ItemCount
		_ = hdr.MessageID
	})
}

func FuzzDecodeChunkHeader(f *testing.F) {
	var seed [HeaderSize]byte
	c := ChunkHeader{
		Magic: MagicChunk, Version: Version, Flags: 0,
		MessageID: 1, TotalMessageLen: 256,
		ChunkIndex: 0, ChunkCount: 3, ChunkPayloadLen: 100,
	}
	c.Encode(seed[:])
	f.Add(seed[:])

	f.Add([]byte{})
	f.Add(make([]byte, 31))
	f.Add(make([]byte, 32))

	f.Fuzz(func(t *testing.T, data []byte) {
		chk, err := DecodeChunkHeader(data)
		if err != nil {
			return
		}
		_ = chk.Magic
		_ = chk.Version
		_ = chk.Flags
		_ = chk.MessageID
		_ = chk.TotalMessageLen
		_ = chk.ChunkIndex
		_ = chk.ChunkCount
		_ = chk.ChunkPayloadLen
	})
}

func FuzzDecodeHello(f *testing.F) {
	var seed [64]byte
	h := Hello{
		LayoutVersion: 1, SupportedProfiles: ProfileBaseline,
		PreferredProfiles:      ProfileBaseline,
		MaxRequestPayloadBytes: 1024, MaxRequestBatchItems: 1,
		MaxResponsePayloadBytes: 1024, MaxResponseBatchItems: 1,
		AuthToken: 0xABCD, PacketSize: 65536,
	}
	h.Encode(seed[:])
	f.Add(seed[:44])

	f.Add([]byte{})
	f.Add(make([]byte, 43))
	f.Add(make([]byte, 44))

	f.Fuzz(func(t *testing.T, data []byte) {
		hello, err := DecodeHello(data)
		if err != nil {
			return
		}
		_ = hello.LayoutVersion
		_ = hello.Flags
		_ = hello.SupportedProfiles
		_ = hello.PreferredProfiles
		_ = hello.MaxRequestPayloadBytes
		_ = hello.MaxRequestBatchItems
		_ = hello.MaxResponsePayloadBytes
		_ = hello.MaxResponseBatchItems
		_ = hello.AuthToken
		_ = hello.PacketSize
	})
}

func FuzzDecodeHelloAck(f *testing.F) {
	var seed [64]byte
	h := HelloAck{
		LayoutVersion: 1, ServerSupportedProfiles: 0x07,
		IntersectionProfiles: 0x05, SelectedProfile: ProfileSHMFutex,
		AgreedMaxRequestPayloadBytes: 2048, AgreedMaxRequestBatchItems: 50,
		AgreedMaxResponsePayloadBytes: 65536, AgreedMaxResponseBatchItems: 1,
		AgreedPacketSize: 32768,
	}
	h.Encode(seed[:])
	f.Add(seed[:48])

	f.Add([]byte{})
	f.Add(make([]byte, 47))
	f.Add(make([]byte, 48))

	f.Fuzz(func(t *testing.T, data []byte) {
		ack, err := DecodeHelloAck(data)
		if err != nil {
			return
		}
		_ = ack.LayoutVersion
		_ = ack.Flags
		_ = ack.ServerSupportedProfiles
		_ = ack.IntersectionProfiles
		_ = ack.SelectedProfile
		_ = ack.AgreedMaxRequestPayloadBytes
		_ = ack.AgreedMaxRequestBatchItems
		_ = ack.AgreedMaxResponsePayloadBytes
		_ = ack.AgreedMaxResponseBatchItems
		_ = ack.AgreedPacketSize
	})
}

func FuzzDecodeCgroupsRequest(f *testing.F) {
	var seed [4]byte
	r := CgroupsRequest{LayoutVersion: 1, Flags: 0}
	r.Encode(seed[:])
	f.Add(seed[:])

	f.Add([]byte{})
	f.Add(make([]byte, 3))
	f.Add(make([]byte, 4))

	f.Fuzz(func(t *testing.T, data []byte) {
		req, err := DecodeCgroupsRequest(data)
		if err != nil {
			return
		}
		_ = req.LayoutVersion
		_ = req.Flags
	})
}

func FuzzDecodeCgroupsResponse(f *testing.F) {
	// Seed: empty snapshot (24 bytes).
	var emptyBuf [4096]byte
	eb := NewCgroupsBuilder(emptyBuf[:], 0, 1, 42)
	emptyTotal := eb.Finish()
	f.Add(emptyBuf[:emptyTotal])

	// Seed: single-item snapshot.
	var singleBuf [4096]byte
	sb := NewCgroupsBuilder(singleBuf[:], 1, 0, 100)
	sb.Add(12345, 0x01, 1, []byte("docker-abc123"), []byte("/sys/fs/cgroup/docker/abc123"))
	singleTotal := sb.Finish()
	f.Add(singleBuf[:singleTotal])

	// Seed: garbage inputs.
	f.Add([]byte{})
	f.Add(make([]byte, 23))
	f.Add(make([]byte, 64))

	f.Fuzz(func(t *testing.T, data []byte) {
		view, err := DecodeCgroupsResponse(data)
		if err != nil {
			return
		}
		// Exercise all items.
		for i := uint32(0); i < view.ItemCount; i++ {
			item, ierr := view.Item(i)
			if ierr != nil {
				continue
			}
			_ = item.LayoutVersion
			_ = item.Flags
			_ = item.Hash
			_ = item.Options
			_ = item.Enabled
			_ = item.Name.Bytes()
			_ = item.Name.Len()
			_ = item.Name.String()
			_ = item.Path.Bytes()
			_ = item.Path.Len()
			_ = item.Path.String()
		}
		// Out-of-bounds item access should not panic.
		_, _ = view.Item(view.ItemCount)
		if view.ItemCount > 0 {
			_, _ = view.Item(view.ItemCount + 1)
		}
	})
}

func FuzzBatchDirDecode(f *testing.F) {
	// Seed: valid 2-entry directory.
	var seed [16]byte
	ne.PutUint32(seed[0:4], 0)   // offset=0, aligned
	ne.PutUint32(seed[4:8], 10)  // length=10
	ne.PutUint32(seed[8:12], 16) // offset=16, aligned
	ne.PutUint32(seed[12:16], 5) // length=5
	f.Add(seed[:], uint32(2), uint32(100))

	// Seed: empty.
	f.Add([]byte{}, uint32(0), uint32(0))
	f.Add(make([]byte, 8), uint32(1), uint32(50))
	f.Add(make([]byte, 4), uint32(1), uint32(50)) // truncated

	f.Fuzz(func(t *testing.T, data []byte, itemCount uint32, packedAreaLen uint32) {
		// Clamp item_count to prevent huge allocations that slow the fuzzer.
		if itemCount > 1024 {
			return
		}
		entries, err := BatchDirDecode(data, itemCount, packedAreaLen)
		if err != nil {
			return
		}
		for _, e := range entries {
			_ = e.Offset
			_ = e.Length
		}
	})
}

func FuzzBatchItemGet(f *testing.F) {
	// Seed: valid single-item batch payload built via BatchBuilder.
	var batchBuf [256]byte
	bb := NewBatchBuilder(batchBuf[:], 2)
	bb.Add([]byte{1, 2, 3, 4, 5})
	bb.Add([]byte{10, 20, 30})
	total, count := bb.Finish()
	f.Add(batchBuf[:total], count, uint32(0))
	f.Add(batchBuf[:total], count, uint32(1))

	f.Add([]byte{}, uint32(0), uint32(0))
	f.Add(make([]byte, 8), uint32(1), uint32(0))

	f.Fuzz(func(t *testing.T, payload []byte, itemCount uint32, index uint32) {
		// Clamp to prevent huge allocations from itemCount.
		if itemCount > 1024 {
			return
		}
		item, err := BatchItemGet(payload, itemCount, index)
		if err != nil {
			return
		}
		_ = len(item)
		if len(item) > 0 {
			_ = item[0]
		}
	})
}
