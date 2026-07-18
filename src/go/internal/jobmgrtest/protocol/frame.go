package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	frameLengthBytes = 4
	MinFrameBytes    = 1
	MaxFrameBytes    = 64 * 1024
)

var (
	ErrEmptyFrame    = errors.New("evaluator protocol: empty frame")
	ErrFrameTooLarge = errors.New("evaluator protocol: frame exceeds 64 KiB")
)

// ReadFrame reads one unsigned-big-endian length-prefixed frame.
func ReadFrame(r io.Reader) ([]byte, error) {
	var header [frameLengthBytes]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("read evaluator frame length: %w", err)
	}
	size := binary.BigEndian.Uint32(header[:])
	if size < MinFrameBytes {
		return nil, ErrEmptyFrame
	}
	if size > MaxFrameBytes {
		return nil, ErrFrameTooLarge
	}
	payload := make([]byte, int(size))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read evaluator frame payload: %w", err)
	}
	return payload, nil
}

// WriteFrame writes one unsigned-big-endian length-prefixed frame.
func WriteFrame(w io.Writer, payload []byte) error {
	if len(payload) < MinFrameBytes {
		return ErrEmptyFrame
	}
	if len(payload) > MaxFrameBytes {
		return ErrFrameTooLarge
	}
	var header [frameLengthBytes]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if err := writeFull(w, header[:]); err != nil {
		return fmt.Errorf("write evaluator frame length: %w", err)
	}
	if err := writeFull(w, payload); err != nil {
		return fmt.Errorf("write evaluator frame payload: %w", err)
	}
	return nil
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if n > 0 {
			payload = payload[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}
