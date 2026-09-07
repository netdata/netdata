// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/klauspost/compress/zstd"
)

const Format = "netdata.snmp_topology.diagnostics"
const Version = 1

var (
	ErrCompressedLimit = errors.New("SNMP topology diagnostic archive compressed-byte limit exceeded")
	ErrDecodedLimit    = errors.New("SNMP topology diagnostic archive decoded-byte limit exceeded")
)

type ReadLimits struct {
	MaxCompressedBytes int64
	MaxDecodedBytes    int64
}

func DefaultReadLimits() ReadLimits { return ReadLimits{128 << 20, 512 << 20} }

// Write encodes one complete document. The caller owns its immutable snapshot.
func Write(w io.Writer, document Document) error {
	if w == nil {
		return errors.New("write SNMP diagnostic archive: nil writer")
	}
	encoder, err := zstd.NewWriter(
		w,
		zstd.WithEncoderLevel(zstd.SpeedDefault),
		zstd.WithEncoderConcurrency(1),
		zstd.WithEncoderCRC(true),
	)
	if err != nil {
		return fmt.Errorf("create diagnostic zstd encoder: %w", err)
	}
	encodeErr := jsonv2.MarshalWrite(
		encoder,
		document,
		jsonv2.JoinOptions(jsonv1.DefaultOptionsV1(), jsontext.EscapeForHTML(false)),
	)
	closeErr := encoder.Close()
	return errors.Join(encodeErr, closeErr)
}

// Read checks the transport and envelope. Domain consumers validate their
// typed sections before inspection or replay.
func Read(r io.Reader, limits ReadLimits) (Document, error) {
	if r == nil {
		return Document{}, errors.New("read SNMP diagnostic archive: nil reader")
	}
	if limits.MaxCompressedBytes <= 0 || limits.MaxCompressedBytes == math.MaxInt64 {
		return Document{}, errors.New("invalid compressed-byte limit")
	}
	if limits.MaxDecodedBytes <= 0 || limits.MaxDecodedBytes == math.MaxInt64 {
		return Document{}, errors.New("invalid decoded-byte limit")
	}
	compressed := &io.LimitedReader{
		R: r,
		N: limits.MaxCompressedBytes + 1,
	}
	decoder, err := zstd.NewReader(compressed, zstd.WithDecoderConcurrency(1))
	if err != nil {
		if compressed.N == 0 {
			return Document{}, ErrCompressedLimit
		}
		return Document{}, fmt.Errorf("create diagnostic zstd decoder: %w", err)
	}
	decoded := &io.LimitedReader{
		R: decoder,
		N: limits.MaxDecodedBytes + 1,
	}
	var document Document
	err = jsonv2.UnmarshalRead(
		decoded,
		&document,
		jsonv2.JoinOptions(jsonv1.DefaultOptionsV1(), jsontext.AllowInvalidUTF8(false)),
	)
	decoder.Close()
	if compressed.N == 0 {
		return Document{}, ErrCompressedLimit
	}
	if decoded.N == 0 {
		return Document{}, ErrDecodedLimit
	}
	if err != nil {
		return Document{}, fmt.Errorf("decode diagnostic JSON: %w", err)
	}
	if document.Format != Format {
		return Document{}, fmt.Errorf("unsupported format %q", document.Format)
	}
	if document.Version != Version {
		return Document{}, fmt.Errorf("unsupported version %d", document.Version)
	}
	return document, nil
}
