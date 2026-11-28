// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"net"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/ssp/credential"
	"github.com/oiweiwei/go-msrpc/ssp/gssapi"
	"github.com/oiweiwei/go-msrpc/ssp/krb5"
	"github.com/oiweiwei/gokrb5.fork/v9/client"
	"github.com/oiweiwei/gokrb5.fork/v9/config"
	"github.com/oiweiwei/gokrb5.fork/v9/credentials"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/etypeID"
	"github.com/oiweiwei/gokrb5.fork/v9/iana/nametype"
	"github.com/oiweiwei/gokrb5.fork/v9/messages"
	"github.com/oiweiwei/gokrb5.fork/v9/types"
)

// maxClockSkew is the Kerberos default (and Active Directory's) tolerance
// between the clocks of the poller, the KDC and the host. A ticket is reused
// only while it stays valid that much past the session deadline, so a host
// whose clock runs ahead still accepts it.
const maxClockSkew = 5 * time.Minute

// kdcDialTimeout bounds each KDC connection attempt. Connecting takes one round
// trip; a domain controller that drops connections, common for the DNS-listed
// controllers of other sites behind firewalls, would otherwise use up the
// whole session deadline before the next KDC is tried.
const kdcDialTimeout = 2 * time.Second

// kerberosEncTypes are the encryption types the poller requests: AES only.
// Windows does not support the SHA-2 variants; RC4 is being retired.
var kerberosEncTypes = []int32{etypeID.AES256_CTS_HMAC_SHA1_96, etypeID.AES128_CTS_HMAC_SHA1_96}

var kerberosEncTypeNames = []string{"aes256-cts-hmac-sha1-96", "aes128-cts-hmac-sha1-96"}

// KerberosAuth authenticates a job's DCOM sessions with Kerberos as one
// domain account. It keeps the service ticket for the job's host and reuses it
// in every session until it nears expiry or the host refuses a session, so
// polls do not contact a KDC. Each new ticket costs one AS and one TGS
// exchange, run inside the session's deadline with a client that is destroyed
// afterwards: no TGT, renewal goroutine or KDC connection outlives it.
type KerberosAuth struct {
	account  string
	realm    string
	password string
	// kdcs are host:port addresses; when empty, the realm's DNS SRV records
	// name them.
	kdcs []string

	// sessionConfig is the Kerberos configuration go-msrpc gets. It names no
	// KDC, so go-msrpc cannot contact one.
	sessionConfig *config.Config

	lookupSRV   func(ctx context.Context, service, proto, name string) (string, []*net.SRV, error)
	dial        func(ctx context.Context, network, address string) (net.Conn, error)
	dialTimeout time.Duration

	ticket *serviceTicket
}

// serviceTicket is a service ticket and its session key. endTime is the KDC's.
type serviceTicket struct {
	spn     string
	client  credentials.Principal
	server  credentials.Principal
	ticket  []byte
	key     types.EncryptionKey
	endTime time.Time
}

// NewKerberosAuth authenticates as account in realm (the uppercase Active
// Directory DNS domain). kdcs are host:port addresses, or empty to look the
// realm's KDCs up in DNS.
func NewKerberosAuth(account, realm, password string, kdcs []string) *KerberosAuth {
	return &KerberosAuth{
		account:       account,
		realm:         realm,
		password:      password,
		kdcs:          kdcs,
		sessionConfig: kerberosConfig(realm, nil, false),
		lookupSRV:     net.DefaultResolver.LookupSRV,
		dial:          (&net.Dialer{}).DialContext,
		dialTimeout:   kdcDialTimeout,
	}
}

// Close forgets the service ticket.
func (k *KerberosAuth) Close() { k.ticket = nil }

func (*KerberosAuth) protocol() string { return "Kerberos" }

// rejected forgets the ticket: the host may have refused it, for example after
// its account key changed or because its clock is further ahead than the
// reuse margin. The next session requests a new one.
func (k *KerberosAuth) rejected() { k.ticket = nil }

func (k *KerberosAuth) bindSecurity(ctx context.Context, host string) ([]dcerpc.Option, error) {
	// Active Directory registers HOST/<name> on the computer account, and
	// DCOM's RPCSS service maps to it.
	spn := "host/" + strings.TrimSuffix(host, ".")
	ticket, err := k.serviceTicket(ctx, spn)
	if err != nil {
		return nil, err
	}
	mechanism := krb5.Mechanism{}
	config := &krb5.Config{
		KRB5Config: k.sessionConfig,
		// DCOM authenticates with three legs and mutual authentication.
		DCEStyle:           true,
		DisablePAFXFAST:    true,
		AnyServiceClassSPN: true,
	}
	creds := credential.NewFromCCache(k.account+"@"+k.realm, ticket.ccache())
	return []dcerpc.Option{
		dcerpc.WithMechanism(mechanism, config),
		dcerpc.WithCredentials(gssapi.NewCredential("", []gssapi.OID{mechanism.Type()}, gssapi.InitiateOnly, creds)),
		dcerpc.WithSecurtyProvider(dcerpc.AuthTypeKerberos),
		dcerpc.WithTargetName(spn),
		dcerpc.WithSeal(),
	}, nil
}

// serviceTicket returns the kept ticket while it stays valid for the session,
// or requests a new one.
func (k *KerberosAuth) serviceTicket(ctx context.Context, spn string) (*serviceTicket, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now()
	}
	if t := k.ticket; t != nil && t.spn == spn && t.endTime.After(deadline.Add(maxClockSkew)) {
		return t, nil
	}
	k.ticket = nil
	t, err := k.requestTicket(ctx, spn)
	if err != nil {
		return nil, err
	}
	k.ticket = t
	return t, nil
}

// requestTicket logs in (AS exchange) and requests the service ticket (TGS
// exchange). The client and its TGT are discarded afterwards.
func (k *KerberosAuth) requestTicket(ctx context.Context, spn string) (*serviceTicket, error) {
	kdcs := k.kdcs
	if len(kdcs) == 0 {
		var err error
		if kdcs, err = k.lookupKDCs(ctx); err != nil {
			return nil, err
		}
	}
	cfg := kerberosConfig(k.realm, kdcs, true)
	exchange := &kdcExchange{
		ctx:         ctx,
		dialTimeout: k.dialTimeout,
		dial:        k.dial,
	}
	cl := client.NewWithPassword(k.account, k.realm, k.password, cfg,
		client.DisablePAFXFAST(true), client.Dialer(exchange))
	// Destroy stops the renewal goroutine of every TGT the exchanges added,
	// including referral TGTs for other realms.
	defer cl.Destroy()

	asReq, err := messages.NewASReqForTGT(k.realm, cfg, cl.Credentials.CName())
	if err != nil {
		return nil, diagnostic("Kerberos login failed: could not build the request")
	}
	asRep, err := cl.ASExchange(k.realm, asReq, 0)
	if err != nil {
		return nil, exchange.failure(phaseLogin)
	}
	if asRep.CRealm == k.realm && exchange.answeredBy != "" {
		// gokrb5 picks a KDC at random for each exchange; the one that issued
		// the TGT is known to answer.
		cfg.Realms[0].KDC = []string{exchange.answeredBy}
	}
	principal := types.NewPrincipalName(nametype.KRB_NT_SRV_INST, spn)
	_, tgsRep, err := cl.TGSREQGenerateAndExchange(principal, k.realm, asRep.Ticket, asRep.DecryptedEncPart.Key, false)
	if err != nil {
		return nil, exchange.failure(phaseServiceTicket)
	}
	raw, err := tgsRep.Ticket.Marshal()
	if err != nil {
		return nil, diagnostic("Kerberos service ticket request failed: could not encode the ticket")
	}
	return &serviceTicket{
		spn: spn,
		client: credentials.Principal{
			Realm:         tgsRep.CRealm,
			PrincipalName: tgsRep.CName,
		},
		server: credentials.Principal{
			Realm:         tgsRep.Ticket.Realm,
			PrincipalName: tgsRep.Ticket.SName,
		},
		ticket:  raw,
		key:     tgsRep.DecryptedEncPart.Key,
		endTime: tgsRep.DecryptedEncPart.EndTime,
	}, nil
}

// lookupKDCs reads the realm's _kerberos._tcp SRV records with the session's
// context; gokrb5's own lookup cannot be canceled.
func (k *KerberosAuth) lookupKDCs(ctx context.Context) ([]string, error) {
	_, records, err := k.lookupSRV(ctx, "kerberos", "tcp", k.realm)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	var kdcs []string
	for _, record := range records {
		if target := strings.TrimSuffix(record.Target, "."); target != "" && record.Port != 0 {
			kdcs = append(kdcs, net.JoinHostPort(target, strconv.Itoa(int(record.Port))))
		}
	}
	if err != nil || len(kdcs) == 0 {
		return nil, diagnostic("Kerberos login failed: DNS has no KDC (_kerberos._tcp SRV record) for the realm")
	}
	return kdcs, nil
}

// ccache hands the ticket to go-msrpc as a credentials cache without a TGT, so
// go-msrpc builds the AP-REQ from it without contacting a KDC. The entry has no
// start or renewal time: the host checks the ticket's own times, and go-msrpc
// must neither reject it for a start time ahead of the poller's clock nor try
// to renew it.
func (t *serviceTicket) ccache() *credentials.CCache {
	return &credentials.CCache{
		Version:          4,
		DefaultPrincipal: t.client,
		Credentials: []*credentials.Credential{{
			Client:  t.client,
			Server:  t.server,
			Key:     t.key,
			EndTime: t.endTime,
			Ticket:  t.ticket,
		}},
	}
}

// kerberosConfig builds the client configuration in code: no krb5.conf is
// read. KDC traffic uses TCP only, so a firewall needs one rule (TCP 88), and
// Active Directory replies carrying a PAC do not hit UDP size limits. kdcs
// serve realm. lookupReferrals lets gokrb5 find the KDCs of another realm in
// DNS when the realm refers a host in another domain of the forest there; that
// lookup cannot be canceled, so only the resolver's own timeouts bound it.
func kerberosConfig(realm string, kdcs []string, lookupReferrals bool) *config.Config {
	cfg := config.New()
	cfg.LibDefaults.DefaultRealm = realm
	cfg.LibDefaults.UDPPreferenceLimit = 1
	cfg.LibDefaults.DNSLookupKDC = lookupReferrals
	cfg.LibDefaults.DefaultTktEnctypes = kerberosEncTypeNames
	cfg.LibDefaults.DefaultTGSEnctypes = kerberosEncTypeNames
	cfg.LibDefaults.PermittedEnctypes = kerberosEncTypeNames
	cfg.LibDefaults.DefaultTktEnctypeIDs = kerberosEncTypes
	cfg.LibDefaults.DefaultTGSEnctypeIDs = kerberosEncTypes
	cfg.LibDefaults.PermittedEnctypeIDs = kerberosEncTypes
	if len(kdcs) != 0 {
		// gokrb5 shuffles the list in place.
		cfg.Realms = []config.Realm{{
			Realm: realm,
			KDC:   slices.Clone(kdcs),
		}}
	}
	return cfg
}
