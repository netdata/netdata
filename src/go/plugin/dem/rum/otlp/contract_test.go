// SPDX-License-Identifier: GPL-3.0-or-later
package otlp

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/config"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/faro"
	"github.com/stretchr/testify/require"
	logs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	traces "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func accepted(b *beacon.Beacon) aggregate.Result {
	return aggregate.Result{
		Accepted:     true,
		Observation:  b,
		Investigated: true,
		PageView:     true,
	}
}

func TestIngestDispositionAndOverflow(t *testing.T) {
	c := newRecCounters()
	l := newExporter(t, "127.0.0.1:1", c, nil)
	tr := newTraceExporter(t, "127.0.0.1:1", "s1", c, nil)
	for _, result := range []aggregate.Result{{}, {Accepted: true}, {Investigated: true}} {
		l.Ingest(tracedBeacon(), result)
		tr.Ingest(tracedBeacon(), result)
	}
	wrong := tracedBeacon()
	wrong.Site = "other"
	l.Ingest(wrong, accepted(wrong))
	tr.Ingest(wrong, accepted(wrong))
	require.Empty(t, l.ch)
	require.Empty(t, tr.spanCh)
	require.Empty(t, c.snapshot())
	// Exercise the actual bounded queues, without replacing their capacities.
	for range queueCap + 1 {
		l.Ingest(tracedBeacon(), accepted(tracedBeacon()))
	}
	for range spanQueueCap + 1 {
		tr.Ingest(tracedBeacon(), accepted(tracedBeacon()))
	}
	require.Len(t, l.ch, queueCap)
	require.Len(t, tr.spanCh, spanQueueCap)
	require.EqualValues(t, 1, c.snapshot()[aggregate.CounterOTLPDropped])
	require.EqualValues(t, 1, c.snapshot()[aggregate.CounterSpansDropped])
}

func TestSharedShutdownDrainsOrDropsUnattempted(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(map[bool]string{false: "drain", true: "expired"}[expired], func(t *testing.T) {
			c := newRecCounters()
			ls, ts := &fakeLogsService{}, &fakeTraceService{}
			l := newExporter(t, startServer(t, ls), c, nil)
			tr := newTraceExporter(t, startTraceServer(t, ts), "s1", c, nil)
			for range 300 {
				l.Ingest(tracedBeacon(), accepted(tracedBeacon()))
				tr.Ingest(tracedBeacon(), accepted(tracedBeacon()))
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			final, cancelFinal := context.WithTimeout(context.Background(), time.Second)
			defer cancelFinal()
			if expired {
				cancelFinal()
			}
			var wg sync.WaitGroup
			wg.Go(func() { l.Run(ctx, func() context.Context { return final }) })
			wg.Go(func() { tr.Run(ctx, func() context.Context { return final }) })
			wg.Wait()
			require.Empty(t, l.ch)
			require.Empty(t, tr.spanCh)
			if expired {
				require.EqualValues(t, 300, c.snapshot()[aggregate.CounterOTLPDropped])
				require.EqualValues(t, 300, c.snapshot()[aggregate.CounterSpansDropped])
				require.Empty(t, ls.requests())
				require.Zero(t, ts.count())
			} else {
				require.EqualValues(t, 300, c.snapshot()[aggregate.CounterOTLPSent])
				require.EqualValues(t, 300, c.snapshot()[aggregate.CounterSpansSent])
			}
		})
	}
}

func TestPartialCountsAndSafeDiagnostics(t *testing.T) {
	var output bytes.Buffer
	previous := exportLog
	exportLog = logger.NewWithWriter(&output)
	t.Cleanup(func() { exportLog = previous })
	for _, n := range []int64{-1, 0, 1, 5, 99} {
		t.Run(string(rune('a'+n+1)), func(t *testing.T) {
			c := newRecCounters()
			ls := &fakeLogsService{
				resp: &logs.ExportLogsServiceResponse{
					PartialSuccess: &logs.ExportLogsPartialSuccess{
						RejectedLogRecords: n,
						ErrorMessage:       "server-secret",
					},
				},
			}
			ts := &fakeTraceService{
				resp: &traces.ExportTraceServiceResponse{
					PartialSuccess: &traces.ExportTracePartialSuccess{
						RejectedSpans: n,
						ErrorMessage:  "server-secret",
					},
				},
			}
			l := newExporter(t, startServer(t, ls), c, nil)
			tr := newTraceExporter(t, startTraceServer(t, ts), "s1", c, nil)
			for range 5 {
				l.Ingest(tracedBeacon(), accepted(tracedBeacon()))
				tr.Ingest(tracedBeacon(), accepted(tracedBeacon()))
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			final := finalContext(t)
			l.Run(ctx, final)
			tr.Run(ctx, final)
			rejected := uint64(max(0, min(5, n)))
			require.EqualValues(t, rejected, c.snapshot()[aggregate.CounterOTLPErrors])
			require.EqualValues(t, 5-rejected, c.snapshot()[aggregate.CounterOTLPSent])
			require.EqualValues(t, rejected, c.snapshot()[aggregate.CounterSpansErrors])
			require.EqualValues(t, 5-rejected, c.snapshot()[aggregate.CounterSpansSent])
		})
	}
	require.Contains(t, output.String(), `logs partial rejection`)
	require.Contains(t, output.String(), `traces partial rejection`)
	require.NotContains(t, output.String(), "server-secret")
}

func TestRepeatedReceiverFailuresHaveBoundedDiagnostics(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "RPC failure", true: "partial rejection"}[partial], func(t *testing.T) {
			var output bytes.Buffer
			previous := exportLog
			exportLog = logger.NewWithWriter(&output)
			t.Cleanup(func() { exportLog = previous })

			lis, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			srv := grpc.NewServer(grpc.UnaryInterceptor(func(
				ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
			) (any, error) {
				if !partial {
					return nil, status.Error(codes.Unavailable, "receiver-secret")
				}
				return handler(ctx, req)
			}))
			logs.RegisterLogsServiceServer(srv, &fakeLogsService{
				resp: &logs.ExportLogsServiceResponse{
					PartialSuccess: &logs.ExportLogsPartialSuccess{
						RejectedLogRecords: 1,
					},
				},
			})
			traces.RegisterTraceServiceServer(srv, &fakeTraceService{
				resp: &traces.ExportTraceServiceResponse{
					PartialSuccess: &traces.ExportTracePartialSuccess{
						RejectedSpans: 1,
					},
				},
			})
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)

			const attempts = 8
			for _, site := range []string{"shop", "checkout"} {
				c := newRecCounters()
				l, err := NewLogs(context.Background(), destination(lis.Addr().String()), site, c, nil)
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, l.Close()) })
				tr := newTraceExporter(t, lis.Addr().String(), site, c, nil)
				b := tracedBeacon()
				b.Site = site
				for range attempts {
					l.Ingest(b, accepted(b))
					tr.Ingest(b, accepted(b))
					l.export(context.Background(), []queued{<-l.ch}, exportTimeout)
					tr.exportSpans(context.Background(), []spanItem{<-tr.spanCh}, exportTimeout)
				}
				// Suppressing repeated diagnostics must not suppress delivery accounting.
				require.EqualValues(t, attempts, c.snapshot()[aggregate.CounterOTLPErrors])
				require.EqualValues(t, attempts, c.snapshot()[aggregate.CounterSpansErrors])
				require.Zero(t, c.snapshot()[aggregate.CounterOTLPSent])
				require.Zero(t, c.snapshot()[aggregate.CounterSpansSent])
			}
			for _, site := range []string{"shop", "checkout"} {
				for _, signal := range []string{"logs", "traces"} {
					prefix := fmt.Sprintf(`site \"%s\" %s `, site, signal)
					require.Equal(t, 1, strings.Count(output.String(), prefix), output.String())
				}
			}
			require.NotContains(t, output.String(), "receiver-secret")
		})
	}
}

func TestConsoleSeverityAndErrorDetails(t *testing.T) {
	e := &Logs{
		transport: &transport{
			redactor: newRedactor(t, "secret-value"),
		},
		now: func() time.Time { return t0 },
	}
	for level, want := range map[string]string{"info": "INFO", "warn": "WARN", "error": "ERROR", "debug": "DEBUG", "trace": "TRACE"} {
		frame, err := json.Marshal(map[string]any{"function": strings.Repeat("é", 3000) + "secret-value", "filename": "app.js"})
		require.NoError(t, err)
		raw, err := json.Marshal(map[string]any{"logs": []any{map[string]any{
			"level": level, "message": "secret-value",
			"context": map[string]any{"type": "secret-value TypeError", "stackFrames": string(frame)},
		}}})
		require.NoError(t, err)
		b, err := faro.Decode(raw, faro.Options{
			Now:         t0,
			ConsoleLogs: true,
			Redactor:    newRedactor(t, "secret-value"),
		})
		require.NoError(t, err)
		recs := e.build(b, false)
		require.Len(t, recs, 1)
		rec := simplify(recs[0])
		require.Equal(t, want, rec.severity)
		require.NotContains(t, rec.body, "secret-value")
		require.Equal(t, "[REDACTED] TypeError", rec.attrs["error.type"])
		require.LessOrEqual(t, len(rec.attrs["error.stack"]), 4096)
	}
}

func tlsFixture(t *testing.T) (tls.Certificate, string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	certPEM, keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: der,
	}), pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})
	dir := t.TempDir()
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(cp, certPEM, 0600))
	require.NoError(t, os.WriteFile(kp, keyPEM, 0600))
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	require.NoError(t, err)
	return pair, cp, kp
}

func useCredentialReader(t *testing.T) {
	t.Helper()
	// Stand in for the credential reader process, preserving real TLS file parsing
	// and gRPC handshakes. Privilege reduction belongs to credentialfile's tests.
	if runtime.GOOS != "windows" {
		dir := t.TempDir()
		exe, err := os.Executable()
		require.NoError(t, err)
		require.NoError(t, os.Symlink(exe, filepath.Join(dir, "nd-run")))
		previous := buildinfo.NetdataBinDir
		buildinfo.NetdataBinDir = dir
		t.Cleanup(func() { buildinfo.NetdataBinDir = previous })
	}
}

func TestBothSignalsTLSAndBearer(t *testing.T) {
	useCredentialReader(t)

	pair, certPath, keyPath := tlsFixture(t)
	for _, tc := range []struct {
		name                                        string
		customCA, clientCert, requireCert, badToken bool
		success                                     bool
	}{
		{name: "TLS custom roots", customCA: true, success: true},
		{name: "mutual TLS", customCA: true, clientCert: true, requireCert: true, success: true},
		{name: "untrusted certificate"},
		{name: "missing client certificate", customCA: true, requireCert: true},
		{name: "rejected bearer", customCA: true, badToken: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string][]string{}
			tlsConfig := &tls.Config{
				Certificates: []tls.Certificate{pair},
				MinVersion:   tls.VersionTLS12,
			}
			if tc.requireCert {
				roots := x509.NewCertPool()
				data, err := os.ReadFile(certPath)
				require.NoError(t, err)
				require.True(t, roots.AppendCertsFromPEM(data))
				tlsConfig.ClientCAs = roots
				tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
			}
			srv := grpc.NewServer(
				grpc.Creds(credentials.NewTLS(tlsConfig)),
				grpc.UnaryInterceptor(
					func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
						md, _ := metadata.FromIncomingContext(ctx)
						mu.Lock()
						seen[info.FullMethod] = append([]string{}, md.Get("authorization")...)
						mu.Unlock()
						if len(md.Get("authorization")) != 1 || md.Get("authorization")[0] != "Bearer synthetic-token" {
							return nil, status.Error(codes.Unauthenticated, "do not echo synthetic-token")
						}
						return handler(ctx, req)
					},
				),
			)
			ls, ts := &fakeLogsService{}, &fakeTraceService{}
			logs.RegisterLogsServiceServer(srv, ls)
			traces.RegisterTraceServiceServer(srv, ts)
			lis, err := net.Listen("tcp", "127.0.0.1:0")
			require.NoError(t, err)
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)
			d := destination("https://" + lis.Addr().String())
			d.AuthToken = "synthetic-token"
			if tc.badToken {
				d.AuthToken = "bad-token"
			}
			if tc.customCA {
				d.TLSCA = certPath
			}
			if tc.clientCert {
				d.TLSCert, d.TLSKey = certPath, keyPath
			}
			c := newRecCounters()
			l, err := NewLogs(context.Background(), d, "s1", c, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = l.Close() })
			tr, err := NewTraces(context.Background(), d, "s1", c, nil)
			require.NoError(t, err)
			t.Cleanup(func() { _ = tr.Close() })
			l.export(context.Background(), l.build(mkBeacon(), true), time.Second)
			tr.exportSpans(context.Background(), []spanItem{{n: 1, rs: tr.resourceSpans(tracedBeacon())}}, time.Second)
			if tc.success {
				require.EqualValues(t, 1, c.snapshot()[aggregate.CounterOTLPSent])
				require.EqualValues(t, 1, c.snapshot()[aggregate.CounterSpansSent])
				mu.Lock()
				require.Len(t, seen, 2)
				for _, auth := range seen {
					require.Equal(t, []string{"Bearer synthetic-token"}, auth)
				}
				mu.Unlock()
			} else {
				require.EqualValues(t, 1, c.snapshot()[aggregate.CounterOTLPErrors])
				require.EqualValues(t, 1, c.snapshot()[aggregate.CounterSpansErrors])
			}
		})
	}
}

func TestActiveTLSFailuresExplainCauseWithoutSecrets(t *testing.T) {
	useCredentialReader(t)
	_, certPath, _ := tlsFixture(t)
	dir := t.TempDir()
	badCA := filepath.Join(dir, "bad-ca.pem")
	badKey := filepath.Join(dir, "bad-key.pem")
	require.NoError(t, os.WriteFile(badCA, []byte("private-material-marker"), 0600))
	require.NoError(
		t,
		os.WriteFile(
			badKey,
			[]byte("-----BEGIN PRIVATE-MATERIAL-MARKER-----\ninvalid\n-----END PRIVATE-MATERIAL-MARKER-----"),
			0600,
		),
	)
	missing := filepath.Join(dir, "missing-diagnostic-secret.pem")
	for name, tc := range map[string]struct {
		ca, cert, key, want string
	}{
		"missing CA":    {ca: missing, want: "could not read certificate"},
		"malformed CA":  {ca: badCA, want: "could not parse any PEM certificates"},
		"missing cert":  {cert: missing, key: badKey, want: "could not read certificate"},
		"missing key":   {cert: certPath, key: missing, want: "could not read key"},
		"malformed key": {cert: certPath, key: badKey, want: "could not parse keypair"},
	} {
		t.Run(name, func(t *testing.T) {
			err := ValidateDestination(context.Background(), config.Destination{
				Endpoint:  ptr("https://localhost:4317"),
				AuthToken: "diagnostic-secret",
				TLSCA:     tc.ca,
				TLSCert:   tc.cert,
				TLSKey:    tc.key,
			})
			require.ErrorContains(t, err, tc.want)
			require.NotContains(t, err.Error(), "diagnostic-secret")
			require.NotContains(t, strings.ToLower(err.Error()), "private-material-marker")
		})
	}
}

func TestValidateActiveTLSMaterial(t *testing.T) {
	for _, d := range []config.Destination{
		{Endpoint: ptr("https://localhost:4317"), TLSCert: "missing"},
		{Endpoint: ptr("http://localhost:4317"), TLSCA: "missing"},
		{Endpoint: ptr("https://localhost:4317"), TLSCA: "missing"},
		{Endpoint: ptr("https://localhost:4317"), TLSCert: "missing", TLSKey: "missing"},
		{Endpoint: ptr("")},
	} {
		require.Error(t, ValidateDestination(context.Background(), d))
	}
	require.NoError(t, ValidateDestination(context.Background(), config.Destination{}))
	require.NoError(t, ValidateDestination(context.Background(), destination("https://localhost:4317")))
}
func ptr(s string) *string { return &s }

func TestMain(m *testing.M) {
	if len(os.Args) == 6 && os.Args[1] == "--file-reader" && os.Args[2] == "read" {
		data, err := os.ReadFile(os.Args[5])
		if err != nil {
			fmt.Fprintln(os.Stderr, "NDFILE 1 2")
			os.Exit(1)
		}
		if _, err = os.Stdout.Write(data); err != nil {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "NDFILE 0 0")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
