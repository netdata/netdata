// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	redact "github.com/netdata/netdata/go/plugins/plugin/dem/internal/redact"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type transport struct {
	siteName string
	conn     *grpc.ClientConn
	counters Counters
	redactor *redact.Redactor
	authMD   metadata.MD
}

func destinationCredentials(
	ctx context.Context,
	d config.Destination,
) (string, credentials.TransportCredentials, error) {
	if errs := config.ValidateDestination(d); len(errs) > 0 {
		return "", nil, fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	endpoint := d.EndpointURL()
	u, _ := url.Parse(endpoint)
	if u.Scheme == "http" {
		return u.Host, insecure.NewCredentials(), nil
	}
	c, err := tlscfg.NewTLSConfig(ctx, tlscfg.TLSConfig{
		TLSCA:   d.TLSCA,
		TLSCert: d.TLSCert,
		TLSKey:  d.TLSKey,
	})
	if err != nil {
		return "", nil, fmt.Errorf("invalid TLS material: %s", redact.NewRedactor(d.AuthToken).Apply(err.Error()))
	}
	if c == nil {
		c = &tls.Config{}
	}
	c.MinVersion = tls.VersionTLS12
	return u.Host, credentials.NewTLS(c), nil
}

// ValidateDestination checks active transport material without dialing or exporting.
func ValidateDestination(ctx context.Context, d config.Destination) error {
	_, _, err := destinationCredentials(ctx, d)
	return err
}

func newTransport(
	ctx context.Context,
	d config.Destination,
	site string,
	counters Counters,
	r *redact.Redactor,
) (*transport, error) {
	target, creds, err := destinationCredentials(ctx, d)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds), grpc.WithConnectParams(grpc.ConnectParams{
		Backoff: backoff.Config{
			BaseDelay:  time.Second,
			Multiplier: 1.6,
			Jitter:     0.2,
			MaxDelay:   60 * time.Second,
		},
		MinConnectTimeout: 5 * time.Second,
	}))
	if err != nil {
		return nil, fmt.Errorf("cannot construct OTLP client")
	}
	t := &transport{
		siteName: site,
		conn:     conn,
		counters: counters,
		redactor: r,
	}
	if d.AuthToken != "" {
		t.authMD = metadata.Pairs("authorization", "Bearer "+d.AuthToken)
	}
	return t, nil
}
func (t *transport) Close() error { return t.conn.Close() }
func (t *transport) redact(s string) string {
	if t.redactor == nil || s == "" {
		return s
	}
	return t.redactor.Apply(s)
}

// Receiver error strings may echo credentials or payloads; log only the status code.
// Limit repeated failures per site/signal/class; counters still account for every record.
func (t *transport) failed(signal string, err error) {
	code := status.Code(err)
	key := t.siteName + "\x00" + signal + "\x00" + code.String()
	exportLog.Limit(key, 1, time.Minute).Warningf(
		"rum/otlp: site %q %s export failed: %s", t.redact(t.siteName), signal, code,
	)
}
func (t *transport) rejected(signal string, n uint64) {
	key := t.siteName + "\x00" + signal + "\x00partial"
	exportLog.Limit(key, 1, time.Minute).Warningf(
		"rum/otlp: site %q %s partial rejection: %d", t.redact(t.siteName), signal, n,
	)
}
func rejectedCount(n int64, total uint64) uint64 {
	if n <= 0 {
		return 0
	}
	if uint64(n) > total {
		return total
	}
	return uint64(n)
}
