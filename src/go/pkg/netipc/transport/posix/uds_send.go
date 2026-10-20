//go:build unix

package posix

import (
	"errors"

	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/transport/internal/framing"
)

// Send sends one logical message. The caller fills Kind, Code, Flags,
// ItemCount, MessageID in hdr; this function sets Magic/Version/
// HeaderLen/PayloadLen. If the total message exceeds PacketSize,
// it is chunked transparently.
func (s *Session) Send(hdr *protocol.Header, payload []byte) error {
	if s.fd < 0 {
		return wrapErr(ErrBadParam, "session closed")
	}

	return framing.SessionSend(framing.SessionSendConfig{
		RoleClient:       s.role == RoleClient,
		PacketSize:       s.PacketSize,
		InflightIDs:      &s.inflightIDs,
		FailAllInflight:  s.failAllInflight,
		IsSendDisconnect: func(err error) bool { return errors.Is(err, ErrSend) },
		SendFirstPacket: func(packetHdr *protocol.Header, packetPayload []byte, _ int) error {
			var hdrBuf [protocol.HeaderSize]byte
			packetHdr.Encode(hdrBuf[:])
			return rawSendIov(s.fd, hdrBuf[:], packetPayload)
		},
		SendChunk: func(chk protocol.ChunkHeader, chunkPayload []byte) error {
			var chkBuf [protocol.HeaderSize]byte
			chk.Encode(chkBuf[:])
			return rawSendIov(s.fd, chkBuf[:], chunkPayload)
		},
		ErrLimitExceeded:  func(msg string) error { return wrapErr(ErrLimitExceeded, msg) },
		ErrDuplicateMsgID: func(msg string) error { return wrapErr(ErrDuplicateMsgID, msg) },
		ErrBadParam:       func(msg string) error { return wrapErr(ErrBadParam, msg) },
	}, hdr, payload)
}
