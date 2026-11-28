// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/oiweiwei/gokrb5.fork/v9/iana/errorcode"
	"github.com/oiweiwei/gokrb5.fork/v9/messages"
)

// maxKDCReply bounds the length a KDC reply may declare. gokrb5 allocates the
// declared length before reading the reply, so a rogue KDC, such as one named
// by a spoofed SRV record, could make the plugin allocate up to 4 GiB. Windows
// limits a Kerberos token, ticket included, to MaxTokenSize (at most 65,535
// bytes), so legitimate replies stay far below this.
const maxKDCReply = 1 << 20

var errKDCReplyTooLarge = errors.New("KDC reply exceeds the size limit")

type kdcPhase string

const (
	phaseLogin         kdcPhase = "Kerberos login"
	phaseServiceTicket kdcPhase = "Kerberos service ticket request"
)

// kdcExchange is the KDC dialer of one ticket request. It binds every KDC
// connection to the session's context and keeps the last KDC reply, because
// gokrb5 reports a KDC's error only as text that also carries the KDC's
// free-form message.
type kdcExchange struct {
	ctx context.Context
	// dialTimeout bounds each connection attempt, so a KDC that drops
	// connections leaves time to try the next one.
	dialTimeout time.Duration
	dial        func(ctx context.Context, network, address string) (net.Conn, error)

	// answered reports that the last KDC contacted sent a reply; reply is that
	// reply's message when it was complete and within maxKDCReply, and
	// answeredBy the KDC's address.
	answered   bool
	reply      []byte
	answeredBy string
}

// Dial implements gokrb5's client.KDCDialer. gokrb5 dials once per KDC
// attempt and returns after the first KDC that answers.
func (e *kdcExchange) Dial(network, address string) (net.Conn, error) {
	e.answered, e.reply, e.answeredBy = false, nil, ""
	ctx, cancel := context.WithTimeout(e.ctx, e.dialTimeout)
	conn, err := e.dial(ctx, network, address)
	cancel()
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(e.ctx, func() { _ = conn.Close() })
	return &kdcConn{
		Conn:     conn,
		exchange: e,
		address:  address,
		stop:     stop,
	}, nil
}

// failure classifies a failed exchange with fixed text and the Kerberos
// error name. The KDC's free-form message and the principal names stay out.
func (e *kdcExchange) failure(phase kdcPhase) error {
	if err := e.ctx.Err(); err != nil {
		return fmt.Errorf("%s: %w", phase, err)
	}
	if !e.answered {
		return diagnostic(string(phase) + " failed: could not reach a KDC")
	}
	if e.reply == nil {
		// Not a Kerberos reply: the connection broke, or a service other than a
		// KDC answered, or the reply exceeded maxKDCReply.
		return diagnostic(string(phase) + " failed: the KDC's reply was incomplete or too large")
	}
	var krbErr messages.KRBError
	if krbErr.Unmarshal(e.reply) == nil {
		return diagnostic(string(phase) + " failed: " + kdcErrorText(krbErr.ErrorCode))
	}
	if phase == phaseLogin {
		// A KDC that does not require preauthentication encrypts its reply
		// with the account's key; gokrb5 also checks the clocks here.
		return diagnostic(string(phase) +
			" failed: the KDC's reply could not be verified (wrong password, or the clocks differ by over five minutes)")
	}
	// After a valid reply, a referral to another domain's KDC can fail
	// without reaching it.
	return diagnostic(string(phase) +
		" failed: the KDC's reply could not be verified, or its referral to another domain could not be followed")
}

// kdcErrorText explains the Kerberos errors an operator can act on.
func kdcErrorText(code int32) string {
	switch code {
	case errorcode.KDC_ERR_C_PRINCIPAL_UNKNOWN:
		return "the account does not exist in the realm (KDC_ERR_C_PRINCIPAL_UNKNOWN)"
	case errorcode.KDC_ERR_S_PRINCIPAL_UNKNOWN:
		return "the realm has no service principal for the host name (KDC_ERR_S_PRINCIPAL_UNKNOWN)"
	case errorcode.KDC_ERR_PREAUTH_FAILED:
		return "wrong password (KDC_ERR_PREAUTH_FAILED)"
	case errorcode.KRB_AP_ERR_SKEW:
		return "the clocks of the poller and the KDC differ by more than the domain allows (KRB_AP_ERR_SKEW)"
	case errorcode.KDC_ERR_ETYPE_NOSUPP:
		return "no AES encryption type in common (KDC_ERR_ETYPE_NOSUPP)"
	case errorcode.KDC_ERR_CLIENT_REVOKED:
		return "the account is disabled, locked out or expired (KDC_ERR_CLIENT_REVOKED)"
	case errorcode.KDC_ERR_KEY_EXPIRED:
		return "the account's password has expired (KDC_ERR_KEY_EXPIRED)"
	case errorcode.KDC_ERR_POLICY:
		return "a domain policy refused the request (KDC_ERR_POLICY)"
	case errorcode.KDC_ERR_WRONG_REALM:
		return "the realm does not hold the account (KDC_ERR_WRONG_REALM)"
	default:
		return "the KDC returned Kerberos error " + strconv.Itoa(int(code))
	}
}

// replied records what the KDC at address sent over TCP: a 4-byte length,
// then the message.
func (e *kdcExchange) replied(address string, received []byte) {
	if len(received) < 4 {
		return
	}
	e.answered, e.answeredBy = true, address
	length := binary.BigEndian.Uint32(received)
	if length <= maxKDCReply && uint64(len(received)-4) >= uint64(length) {
		e.reply = received[4 : 4+length]
	}
}

type kdcConn struct {
	net.Conn
	exchange *kdcExchange
	address  string
	stop     func() bool
	received []byte
}

// SetDeadline keeps gokrb5's fixed per-KDC deadline within the session's.
func (c *kdcConn) SetDeadline(t time.Time) error {
	if deadline, ok := c.exchange.ctx.Deadline(); ok && deadline.Before(t) {
		t = deadline
	}
	return c.Conn.SetDeadline(t)
}

func (c *kdcConn) Read(b []byte) (int, error) {
	var n int
	var err error
	if missing := 4 - len(c.received); missing > 0 && len(b) >= missing {
		// gokrb5 reads the length prefix with one Read and trusts whatever
		// arrived, so the prefix is read whole before it is checked.
		n, err = io.ReadFull(c.Conn, b[:missing])
	} else {
		n, err = c.Conn.Read(b)
	}
	c.received = append(c.received, b[:n]...)
	if len(c.received) >= 4 && binary.BigEndian.Uint32(c.received) > maxKDCReply {
		return n, errKDCReplyTooLarge
	}
	return n, err
}

func (c *kdcConn) Close() error {
	c.stop()
	c.exchange.replied(c.address, c.received)
	return c.Conn.Close()
}
