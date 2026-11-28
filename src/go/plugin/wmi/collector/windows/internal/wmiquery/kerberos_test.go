// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"runtime"
	"runtime/pprof"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jcmturner/gofork/encoding/asn1"
	"github.com/oiweiwei/go-msrpc/dcerpc"
	activation "github.com/oiweiwei/go-msrpc/msrpc/dcom/iactivation/v0"
	"github.com/oiweiwei/go-msrpc/ssp/credential"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/krb5"
	"github.com/oiweiwei/gokrb5.fork/v9/crypto"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/errorcode"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/etypeID"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/flags"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/keyusage"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/msgtype"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/nametype"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/patype"
	"github.com/oiweiwei/gokrb5.fork/v9/keytab"
	"github.com/oiweiwei/gokrb5.fork/v9/messages"
	"github.com/oiweiwei/gokrb5.fork/v9/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRealm    = "EXAMPLE.TEST"
	testAccount  = "reader"
	testPassword = "synthetic-password"
	testHost     = "server.example.test"
	testSPN      = "host/" + testHost
	// kdcMarker stands for the free-form text and names a KDC error carries;
	// it must never reach a diagnostic.
	kdcMarker = "kdc-free-text-marker"
)

func TestKerberosReusesTheServiceTicketAcrossSessions(t *testing.T) {
	kdc := newTestKDC(t)
	auth := kdc.auth(testPassword)

	// The first session logs in and requests the ticket; a trailing dot in the
	// host name is not part of the service principal. Like Active Directory,
	// the KDC requires preauthentication: a ticket costs two AS requests and
	// one TGS request.
	authenticateSession(t, kdc, auth, testHost+".")
	assert.Equal(t, kdcCounts{
		as:  2,
		tgs: 1,
	}, kdc.counts())
	assert.Equal(t, []string{testSPN}, kdc.requestedSPNs())

	// Later sessions reuse it without contacting the KDC.
	authenticateSession(t, kdc, auth, testHost)
	authenticateSession(t, kdc, auth, testHost)
	assert.Equal(t, kdcCounts{
		as:  2,
		tgs: 1,
	}, kdc.counts())

	// A ticket that would expire within the clock-skew margin after the
	// session deadline is replaced.
	auth.ticket.endTime = time.Now().Add(maxClockSkew)
	authenticateSession(t, kdc, auth, testHost)
	assert.Equal(t, kdcCounts{
		as:  4,
		tgs: 2,
	}, kdc.counts())

	// A host that refused a session gets a new ticket next time.
	auth.rejected()
	authenticateSession(t, kdc, auth, testHost)
	assert.Equal(t, kdcCounts{
		as:  6,
		tgs: 3,
	}, kdc.counts())

	auth.Close()
	assert.Nil(t, auth.ticket)

	// No gokrb5 client outlived its request, such as a TGT renewal goroutine.
	var goroutines bytes.Buffer
	require.NoError(t, pprof.Lookup("goroutine").WriteTo(&goroutines, 1))
	assert.NotContains(t, goroutines.String(), "gokrb5.fork/v9/client.")
}

func TestKerberosFindsTheKDCsInDNS(t *testing.T) {
	kdc := newTestKDC(t)
	auth := kdc.auth(testPassword)
	host, port, err := net.SplitHostPort(kdc.address)
	require.NoError(t, err)
	portNumber, err := strconv.ParseUint(port, 10, 16)
	require.NoError(t, err)
	var looked []string
	auth.kdcs = nil
	auth.lookupSRV = func(_ context.Context, service, proto, name string) (string, []*net.SRV, error) {
		looked = append(looked, service+"/"+proto+"/"+name)
		return "", []*net.SRV{{
			Target: host + ".",
			Port:   uint16(portNumber),
		}}, nil
	}

	authenticateSession(t, kdc, auth, testHost)
	assert.Equal(t, []string{"kerberos/tcp/" + testRealm}, looked)
	assert.Equal(t, kdcCounts{
		as:  2,
		tgs: 1,
	}, kdc.counts())
}

func TestKerberosDiagnostics(t *testing.T) {
	tests := map[string]struct {
		prepare func(*testKDC, *KerberosAuth)
		want    string
	}{
		"unknown account": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KDC_ERR_C_PRINCIPAL_UNKNOWN },
			want:    "Kerberos login failed: the account does not exist in the realm (KDC_ERR_C_PRINCIPAL_UNKNOWN)",
		},
		"wrong password": {
			prepare: func(_ *testKDC, auth *KerberosAuth) { auth.password = "another-password" },
			want:    "Kerberos login failed: wrong password (KDC_ERR_PREAUTH_FAILED)",
		},
		"wrong password without preauthentication": {
			prepare: func(kdc *testKDC, auth *KerberosAuth) {
				kdc.noPreauth = true
				auth.password = "another-password"
			},
			want: "Kerberos login failed: the KDC's reply could not be verified " +
				"(wrong password, or the clocks differ by over five minutes)",
		},
		"clock skew": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KRB_AP_ERR_SKEW },
			want: "Kerberos login failed: the clocks of the poller and the KDC differ by more than the domain " +
				"allows (KRB_AP_ERR_SKEW)",
		},
		"encryption type mismatch": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KDC_ERR_ETYPE_NOSUPP },
			want:    "Kerberos login failed: no AES encryption type in common (KDC_ERR_ETYPE_NOSUPP)",
		},
		"disabled account": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KDC_ERR_CLIENT_REVOKED },
			want:    "Kerberos login failed: the account is disabled, locked out or expired (KDC_ERR_CLIENT_REVOKED)",
		},
		"expired password": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KDC_ERR_KEY_EXPIRED },
			want:    "Kerberos login failed: the account's password has expired (KDC_ERR_KEY_EXPIRED)",
		},
		"other KDC error": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.asError = errorcode.KDC_ERR_BADOPTION },
			want:    "Kerberos login failed: the KDC returned Kerberos error 13",
		},
		"unknown service principal": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.tgsError = errorcode.KDC_ERR_S_PRINCIPAL_UNKNOWN },
			want: "Kerberos service ticket request failed: the realm has no service principal for the host name " +
				"(KDC_ERR_S_PRINCIPAL_UNKNOWN)",
		},
		"KDC unreachable": {
			prepare: func(_ *testKDC, auth *KerberosAuth) { auth.kdcs = []string{closedAddress(t)} },
			want:    "Kerberos login failed: could not reach a KDC",
		},
		"no KDC in DNS": {
			prepare: func(_ *testKDC, auth *KerberosAuth) {
				auth.kdcs = nil
				auth.lookupSRV = func(context.Context, string, string, string) (string, []*net.SRV, error) {
					return "", nil, &net.DNSError{
						Err:        "no such host",
						Name:       kdcMarker,
						IsNotFound: true,
					}
				}
			},
			want: "Kerberos login failed: DNS has no KDC (_kerberos._tcp SRV record) for the realm",
		},
		"oversized reply": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.declareLength = maxKDCReply + 1 },
			want:    "Kerberos login failed: the KDC's reply was incomplete or too large",
		},
		"truncated reply": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.truncate = true },
			want:    "Kerberos login failed: the KDC's reply was incomplete or too large",
		},
		"service ticket from an unreachable KDC": {
			prepare: func(kdc *testKDC, _ *KerberosAuth) { kdc.closeAfterLogin = true },
			want:    "Kerberos service ticket request failed: could not reach a KDC",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			kdc := newTestKDC(t)
			auth := kdc.auth(testPassword)
			tc.prepare(kdc, auth)

			_, err := auth.bindSecurity(sessionContext(t, 3*time.Second), testHost)
			require.Error(t, err)
			assert.Equal(t, tc.want, SafeMessage(err))
			assert.NotContains(t, err.Error(), kdcMarker)
			assert.NotContains(t, err.Error(), testAccount)
			assert.Nil(t, auth.ticket)
		})
	}
}

func TestKerberosKDCExchangeStaysWithinTheSessionDeadline(t *testing.T) {
	kdc := newTestKDC(t)
	kdc.silent = true
	auth := kdc.auth(testPassword)

	start := time.Now()
	_, err := auth.bindSecurity(sessionContext(t, 200*time.Millisecond), testHost)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, "WMI collection timed out", SafeMessage(err))
	// gokrb5 alone would wait five seconds for the silent KDC.
	assert.Less(t, time.Since(start), 2*time.Second)
}

func TestKerberosReadsASplitReplyLengthWhole(t *testing.T) {
	t.Run("valid reply", func(t *testing.T) {
		kdc := newTestKDC(t)
		kdc.splitHeader = true
		authenticateSession(t, kdc, kdc.auth(testPassword), testHost)
	})
	t.Run("oversized reply", func(t *testing.T) {
		kdc := newTestKDC(t)
		kdc.splitHeader = true
		kdc.declareLength = 0xF0000000
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		_, err := kdc.auth(testPassword).bindSecurity(sessionContext(t, 3*time.Second), testHost)
		runtime.ReadMemStats(&after)
		assert.Equal(t, "Kerberos login failed: the KDC's reply was incomplete or too large", SafeMessage(err))
		// The declared length is never allocated.
		assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<20))
	})
}

func TestKerberosTriesTheNextKDCWhenOneDropsConnections(t *testing.T) {
	const unreachable = "dropped.example.test:88"
	for range 10 {
		kdc := newTestKDC(t)
		auth := NewKerberosAuth(testAccount, testRealm, testPassword, []string{unreachable, kdc.address})
		auth.dialTimeout = 50 * time.Millisecond
		var afterLogin int
		auth.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			if address == unreachable {
				if kdc.counts().as == 2 {
					afterLogin++
				}
				// A firewall that drops the connection attempt.
				<-ctx.Done()
				return nil, ctx.Err()
			}
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}
		_, err := auth.bindSecurity(sessionContext(t, time.Second), testHost)
		require.NoError(t, err)
		assert.Equal(t, kdcCounts{
			as:  2,
			tgs: 1,
		}, kdc.counts())
		// The service ticket request goes to the KDC that issued the TGT.
		assert.Zero(t, afterLogin)
	}
}

func TestKerberosAuthenticationFailureNamesTheProtocol(t *testing.T) {
	err := gssapi.NewError(gssapi.Failure, errors.New("krb5: "+kdcMarker))
	assert.Equal(
		t,
		"bind: Kerberos authentication failed",
		SafeMessage(rpcFailure(t.Context(), "Kerberos", "bind", err)),
	)
}

func TestDCOMForgetsTheTicketOnlyWhenTheHostWasReached(t *testing.T) {
	for name, tc := range map[string]struct {
		dial       func(context.Context, string, string) (net.Conn, error)
		wantTicket bool
	}{
		"host unreachable": {
			dial: func(context.Context, string, string) (net.Conn, error) {
				return nil, syscall.ECONNREFUSED
			},
			wantTicket: true,
		},
		"host refused the session": {
			dial: func(context.Context, string, string) (net.Conn, error) {
				client, peer := net.Pipe()
				// The host reads the bind and closes the connection.
				go func() {
					_, _ = peer.Read(make([]byte, 1))
					_ = peer.Close()
				}()
				return fixedRemoteAddr{
					Conn: client,
				}, nil
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			kdc := newTestKDC(t)
			auth := kdc.auth(testPassword)
			ctx := sessionContext(t, 3*time.Second)
			_, err := auth.bindSecurity(ctx, testHost)
			require.NoError(t, err)
			require.NotNil(t, auth.ticket)

			ctx, cancel := context.WithCancel(ctx)
			session := &DCOMSession{
				ctx:    ctx,
				cancel: cancel,
				auth:   auth.protocol(),
				dialer: rpcDialer{
					host: testHost,
					ctx:  ctx,
					dial: tc.dial,
				},
			}
			require.Error(t, session.open(testHost, auth))
			session.Close()
			assert.Equal(t, tc.wantTicket, auth.ticket != nil)
			assert.Equal(t, kdcCounts{
				as:  2,
				tgs: 1,
			}, kdc.counts())
		})
	}
}

// authenticateSession runs the security exchange of one DCOM bind to host:
// the go-msrpc initiator with the options the session would use, and the
// go-msrpc Kerberos acceptor with the host's key.
func authenticateSession(t *testing.T, kdc *testKDC, auth *KerberosAuth, host string) {
	t.Helper()
	ctx := sessionContext(t, 3*time.Second)
	opts, err := auth.bindSecurity(ctx, host)
	require.NoError(t, err)
	parsed, err := dcerpc.ParseOptions(ctx, append(opts, dcerpc.WithAbstractSyntax(activation.ActivationSyntaxV0_0))...)
	require.NoError(t, err)
	client := parsed.Security
	require.Equal(t, dcerpc.AuthTypeKerberos, client.Type)
	require.Equal(t, dcerpc.AuthLevelPktPrivacy, client.Level)

	server := gssapi.NewSecurityContext(ctx,
		gssapi.WithCredential(credential.NewFromKeytab(testSPN, kdc.serviceKeys)),
		gssapi.WithMechanismFactory(krb5.Mechanism{}, &krb5.Config{
			DCEStyle: true,
		}),
	)

	apReq, err := client.Init(ctx, nil)
	require.NoError(t, err)
	require.False(t, client.Established(), "DCOM uses three-leg mutual authentication")
	apRep, err := gssapi.AcceptSecurityContext(
		server,
		&gssapi.Token{
			Payload: apReq,
		},
		gssapi.WithMechanismType(krb5.MechanismType),
	)
	require.NoError(t, err)
	final, err := client.Init(ctx, apRep.Payload)
	require.NoError(t, err)
	require.True(t, client.Established())
	_, err = gssapi.AcceptSecurityContext(
		server,
		&gssapi.Token{
			Payload: final,
		},
		gssapi.WithMechanismType(krb5.MechanismType),
	)
	require.NoError(t, err)
}

func sessionContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	t.Cleanup(cancel)
	return ctx
}

func closedAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

type kdcCounts struct{ as, tgs int }

// testKDC is a minimal KDC over TCP that issues real tickets: a TGT for the
// account and a service ticket for any requested service, encrypted with the
// keys in serviceKeys. Like Active Directory, it requires preauthentication:
// an AS request without an encrypted timestamp gets KDC_ERR_PREAUTH_REQUIRED.
type testKDC struct {
	t           *testing.T
	address     string
	listener    net.Listener
	krbtgtKeys  *keytab.Keytab
	serviceKeys *keytab.Keytab

	// asError and tgsError make the exchange fail with that Kerberos error;
	// noPreauth answers AS requests without preauthentication; silent leaves
	// requests unanswered; declareLength sends a reply header declaring that
	// length and nothing else; truncate sends half the reply; splitHeader sends
	// the reply length one byte at a time; closeAfterLogin stops accepting
	// connections once it issued a TGT.
	asError         int32
	tgsError        int32
	noPreauth       bool
	silent          bool
	declareLength   uint32
	truncate        bool
	splitHeader     bool
	closeAfterLogin bool

	mu         sync.Mutex
	n          kdcCounts
	spns       []string
	sessionKey types.EncryptionKey
}

func newTestKDC(t *testing.T) *testKDC {
	t.Helper()
	kdc := &testKDC{
		t:           t,
		krbtgtKeys:  keytab.New(),
		serviceKeys: keytab.New(),
	}
	now := time.Now()
	require.NoError(t, kdc.krbtgtKeys.AddEntry("krbtgt/"+testRealm, testRealm, "synthetic-krbtgt", now, 1,
		etypeID.AES256_CTS_HMAC_SHA1_96))
	require.NoError(t, kdc.serviceKeys.AddEntry(testSPN, testRealm, "synthetic-host-key", now, 1,
		etypeID.AES256_CTS_HMAC_SHA1_96))

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	kdc.address = listener.Addr().String()
	kdc.listener = listener
	var wg sync.WaitGroup
	t.Cleanup(func() {
		_ = listener.Close()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			wg.Go(func() { kdc.serve(conn) })
		}
	})
	return kdc
}

func (kdc *testKDC) auth(password string) *KerberosAuth {
	return NewKerberosAuth(testAccount, testRealm, password, []string{kdc.address})
}

func (kdc *testKDC) counts() kdcCounts {
	kdc.mu.Lock()
	defer kdc.mu.Unlock()
	return kdc.n
}

func (kdc *testKDC) requestedSPNs() []string {
	kdc.mu.Lock()
	defer kdc.mu.Unlock()
	return kdc.spns
}

func (kdc *testKDC) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return
	}
	request := make([]byte, binary.BigEndian.Uint32(header[:]))
	if _, err := io.ReadFull(conn, request); err != nil {
		return
	}
	if kdc.silent {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	reply := kdc.reply(request)
	length := uint32(len(reply))
	if kdc.declareLength != 0 {
		length, reply = kdc.declareLength, nil
	}
	if kdc.truncate {
		reply = reply[:len(reply)/2]
	}
	binary.BigEndian.PutUint32(header[:], length)
	if kdc.splitHeader {
		for _, b := range header {
			_, _ = conn.Write([]byte{b})
			time.Sleep(20 * time.Millisecond)
		}
	} else {
		_, _ = conn.Write(header[:])
	}
	_, _ = conn.Write(reply)
}

// reply runs on the KDC's goroutine, so failures are reported with t.Error
// and answered with nothing.
func (kdc *testKDC) reply(request []byte) []byte {
	var as messages.ASReq
	if as.Unmarshal(request) == nil {
		kdc.mu.Lock()
		kdc.n.as++
		kdc.mu.Unlock()
		if kdc.asError != 0 {
			return kdc.krbError(kdc.asError)
		}
		krbtgt := types.NewPrincipalName(nametype.KRB_NT_SRV_INST, "krbtgt/"+testRealm)
		userKey, _, err := crypto.GetKeyFromPassword(testPassword, as.ReqBody.CName, testRealm,
			etypeID.AES256_CTS_HMAC_SHA1_96, types.PADataSequence{})
		if err != nil {
			kdc.t.Error(err)
			return nil
		}
		if !kdc.noPreauth {
			if reply, ok := kdc.preauthenticate(as, userKey); !ok {
				return reply
			}
		}
		if kdc.closeAfterLogin {
			_ = kdc.listener.Close()
		}
		reply, sessionKey := kdc.issue(as.ReqBody.CName, krbtgt, as.ReqBody.Nonce, kdc.krbtgtKeys, userKey,
			keyusage.AS_REP_ENCPART, msgtype.KRB_AS_REP)
		kdc.mu.Lock()
		kdc.sessionKey = sessionKey
		kdc.mu.Unlock()
		return reply
	}
	var tgs messages.TGSReq
	if err := tgs.Unmarshal(request); err != nil {
		kdc.t.Error(err)
		return nil
	}
	kdc.mu.Lock()
	kdc.n.tgs++
	kdc.spns = append(kdc.spns, tgs.ReqBody.SName.PrincipalNameString())
	tgtKey := kdc.sessionKey
	kdc.mu.Unlock()
	if kdc.tgsError != 0 {
		return kdc.krbError(kdc.tgsError)
	}
	reply, _ := kdc.issue(tgs.ReqBody.CName, tgs.ReqBody.SName, tgs.ReqBody.Nonce, kdc.serviceKeys, tgtKey,
		keyusage.TGS_REP_ENCPART_SESSION_KEY, msgtype.KRB_TGS_REP)
	return reply
}

// issue builds an AS or TGS reply with a ten-hour ticket for sname, encrypted
// with sname's key from keys; replyKey encrypts the reply's secret part.
func (kdc *testKDC) issue(
	cname, sname types.PrincipalName,
	nonce int,
	keys *keytab.Keytab,
	replyKey types.EncryptionKey,
	usage uint32,
	msgType int,
) ([]byte, types.EncryptionKey) {
	now := time.Now().UTC().Truncate(time.Second)
	ticketFlags := types.NewKrbFlags()
	types.SetFlag(&ticketFlags, flags.Initial)
	ticket, sessionKey, err := messages.NewTicket(cname, testRealm, sname, testRealm, ticketFlags, keys,
		etypeID.AES256_CTS_HMAC_SHA1_96, 1, now, now, now.Add(10*time.Hour), time.Time{})
	if err != nil {
		kdc.t.Error(err)
		return nil, sessionKey
	}
	part := messages.EncKDCRepPart{
		Key:       sessionKey,
		LastReqs:  []messages.LastReq{},
		Nonce:     nonce,
		Flags:     ticketFlags,
		AuthTime:  now,
		StartTime: now,
		EndTime:   now.Add(10 * time.Hour),
		SRealm:    testRealm,
		SName:     sname,
	}
	plain, err := part.Marshal()
	if err != nil {
		kdc.t.Error(err)
		return nil, sessionKey
	}
	encrypted, err := crypto.GetEncryptedData(plain, replyKey, usage, 1)
	if err != nil {
		kdc.t.Error(err)
		return nil, sessionKey
	}
	fields := messages.KDCRepFields{
		PVNO:    5,
		MsgType: msgType,
		CRealm:  testRealm,
		CName:   cname,
		Ticket:  ticket,
		EncPart: encrypted,
	}
	var reply []byte
	if msgType == msgtype.KRB_AS_REP {
		rep := messages.ASRep{
			KDCRepFields: fields,
		}
		reply, err = rep.Marshal()
	} else {
		rep := messages.TGSRep{
			KDCRepFields: fields,
		}
		reply, err = rep.Marshal()
	}
	if err != nil {
		kdc.t.Error(err)
	}
	return reply, sessionKey
}

// preauthenticate answers an AS request without a valid encrypted timestamp
// as Active Directory does: KDC_ERR_PREAUTH_REQUIRED with the account's
// encryption type and salt, or KDC_ERR_PREAUTH_FAILED for a wrong key.
func (kdc *testKDC) preauthenticate(as messages.ASReq, userKey types.EncryptionKey) ([]byte, bool) {
	for _, pa := range as.PAData {
		if pa.PADataType != patype.PA_ENC_TIMESTAMP {
			continue
		}
		var timestamp types.EncryptedData
		if timestamp.Unmarshal(pa.PADataValue) == nil {
			if _, err := crypto.DecryptEncPart(timestamp, userKey, keyusage.AS_REQ_PA_ENC_TIMESTAMP); err == nil {
				return nil, true
			}
		}
		return kdc.krbError(errorcode.KDC_ERR_PREAUTH_FAILED), false
	}
	info, err := asn1.Marshal([]types.ETypeInfo2Entry{{
		EType: etypeID.AES256_CTS_HMAC_SHA1_96,
		Salt:  testRealm + testAccount,
	}})
	if err != nil {
		kdc.t.Error(err)
		return nil, false
	}
	data, err := asn1.Marshal(types.PADataSequence{{
		PADataType:  patype.PA_ETYPE_INFO2,
		PADataValue: info,
	}})
	if err != nil {
		kdc.t.Error(err)
		return nil, false
	}
	krbErr := messages.NewKRBError(types.NewPrincipalName(nametype.KRB_NT_SRV_INST, "krbtgt/"+testRealm), testRealm,
		errorcode.KDC_ERR_PREAUTH_REQUIRED, kdcMarker)
	krbErr.EData = data
	b, err := krbErr.Marshal()
	if err != nil {
		kdc.t.Error(err)
	}
	return b, false
}

// krbError carries the marker where a KDC puts free-form text and names.
func (kdc *testKDC) krbError(code int32) []byte {
	krbErr := messages.NewKRBError(types.NewPrincipalName(nametype.KRB_NT_PRINCIPAL, kdcMarker), testRealm, code,
		kdcMarker)
	krbErr.CName = types.NewPrincipalName(nametype.KRB_NT_PRINCIPAL, testAccount)
	krbErr.CRealm = testRealm
	b, err := krbErr.Marshal()
	if err != nil {
		kdc.t.Error(err)
	}
	return b
}
