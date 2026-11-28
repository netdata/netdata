// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

const emptyMetricDefinitions = `{"value":[]}`

type wireDefinitionRead struct {
	method, path, host, authorization string
	contentLength, transferEncoding   string
	body                              string
}

func TestARMTransportChecksAdmissionOnAutomaticGETReplay(t *testing.T) {
	for _, protocol := range []string{"HTTP/1.1", "HTTP/2"} {
		for _, held := range []bool{false, true} {
			name := "replay allowed"
			if held {
				name = "replay held"
			}
			t.Run(protocol+"/"+name, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
				defer cancel()
				reads := make(chan wireDefinitionRead, 2)
				interrupt := make(chan struct{})
				replay := func() bool {
					select {
					case <-interrupt:
						return true
					case <-ctx.Done():
						return false
					}
				}
				client := definitionReplayClient(t, ctx, protocol, reads, replay)
				sdk, err := arm.NewClient("test", "v1.0.0", new(fake.TokenCredential), &arm.ClientOptions{
					ClientOptions: withAzureRequestTransport(azcore.ClientOptions{Transport: client}),
				})
				require.NoError(t, err)
				batch := testWireEvidenceBatch("A")
				key := definitionKey{batch.Resources[0].ID, batch.Profile.MetricNamespace}
				p := queryPoll{
					c:      &Collector{now: time.Now},
					state:  &recoveryState{advice: map[definitionKey]*definitionAdvice{key.canonical(): {}}},
					events: make(chan queryEvent, 1),
				}
				op := p.takeOperation()
				op.read = &adviceRequest{key: key.canonical(), subscription: batch.SubscriptionID}
				done := make(chan error, 1)
				go func() {
					_, err := (&metricDefinitionsClient{sdk}).list(context.WithValue(ctx, queryOperationKey{}, op), key)
					done <- err
				}()

				var attempts int
				var sent []wireDefinitionRead
			loop:
				for {
					select {
					case e := <-p.events:
						attempts++
						p.onEvent(e)
					case got := <-reads:
						sent = append(sent, got)
						if len(sent) == 1 {
							if held {
								// Another read received a subscription-wide hold before
								// the first attempt failed and became eligible to replay.
								other := &adviceRequest{
									key:          definitionKey{resourceID: key.resourceID + "-other", namespace: key.namespace},
									subscription: batch.SubscriptionID,
								}
								headers := make(http.Header)
								headers.Set("X-Ms-Ratelimit-Remaining-Subscription-Reads", "0")
								p.onEvent(queryEvent{
									op:      &queryOperation{read: other},
									receipt: throttleReceipt{status: http.StatusTooManyRequests, headers: headers, at: time.Now()},
								})
							}
							close(interrupt)
						}
					case err = <-done:
						break loop
					case <-ctx.Done():
						t.Fatal(ctx.Err())
					}
				}
				for len(reads) > 0 { // The peer reports each request before sending the response.
					sent = append(sent, <-reads)
				}
				require.NotEmpty(t, sent)
				assert.Equal(t, 2, attempts, "initial send and transport replay each require a decision")
				if held {
					assert.ErrorIs(t, err, errHeldAzureRequest)
					assert.Len(t, sent, 1)
				} else {
					assert.NoError(t, err)
					assert.Len(t, sent, 2)
				}
				for _, req := range sent {
					assert.Equal(t, wireDefinitionRead{
						method:        http.MethodGet,
						path:          key.resourceID + "/providers/Microsoft.Insights/metricDefinitions?api-version=2023-10-01&metricnamespace=Microsoft.Cdn%2Fprofiles",
						host:          "management.azure.com",
						authorization: "Bearer fake_token",
					}, req)
				}
			})
		}
	}
}

func definitionReplayClient(t *testing.T, ctx context.Context, protocol string, reads chan<- wireDefinitionRead, replay func() bool) *http.Client {
	t.Helper()
	if protocol == "HTTP/2" {
		return replayHTTP2Client(t, ctx, func(peer net.Conn) {
			serveDefinitionReplayHTTP2(peer, reads, replay)
		})
	}
	var peers sync.WaitGroup
	dial := func(context.Context, string, string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, peer := net.Pipe()
		deadline, _ := ctx.Deadline()
		_ = peer.SetDeadline(deadline)
		peers.Go(func() {
			defer peer.Close()
			serveDefinitionReplayHTTP1(peer, reads, replay)
		})
		return client, nil
	}
	rt := &http.Transport{DialTLSContext: dial}
	t.Cleanup(func() { rt.CloseIdleConnections(); peers.Wait() })
	client := &http.Client{Transport: rt}
	// Go retries a transmitted HTTP/1.1 GET after EOF on a reused connection.
	warmup, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://management.azure.com/warmup", nil)
	require.NoError(t, err)
	resp, err := client.Do(warmup)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	return client
}

func serveDefinitionReplayHTTP1(peer net.Conn, reads chan<- wireDefinitionRead, replay func() bool) {
	reader := bufio.NewReader(peer)
	for warmed := false; ; {
		req, err := http.ReadRequest(reader)
		if err != nil {
			return
		}
		if req.URL.Path == "/warmup" {
			warmed = true
			_, _ = io.WriteString(peer, "HTTP/1.1 204 No Content\r\n\r\n")
			continue
		}
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return
		}
		reads <- wireDefinitionRead{
			method:           req.Method,
			path:             req.URL.RequestURI(),
			host:             req.Host,
			authorization:    req.Header.Get("Authorization"),
			contentLength:    req.Header.Get("Content-Length"),
			transferEncoding: strings.Join(req.TransferEncoding, ","),
			body:             string(body),
		}
		if warmed {
			replay() // EOF after the server has read the entire request.
			return
		}
		_, _ = fmt.Fprintf(peer, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(emptyMetricDefinitions), emptyMetricDefinitions)
	}
}

func replayHTTP2Client(t *testing.T, ctx context.Context, serve func(net.Conn)) *http.Client {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{"management.azure.com", "eastus.metrics.monitor.azure.com"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	require.NoError(t, err)
	signed, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	ca := x509.NewCertPool()
	ca.AddCert(signed)
	serverTLS := &tls.Config{
		NextProtos:   []string{"h2"},
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}},
	}

	rt := newAzureRequestHTTPTransport()
	rt.Proxy = nil
	rt.TLSClientConfig.RootCAs = ca
	var peers sync.WaitGroup
	rt.DialTLSContext = func(dialCtx context.Context, _, addr string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tlsCfg := rt.TLSClientConfig.Clone()
		tlsCfg.ServerName, _, _ = net.SplitHostPort(addr)
		client, peer := net.Pipe()
		deadline, _ := ctx.Deadline()
		_ = peer.SetDeadline(deadline)
		peers.Go(func() {
			defer peer.Close()
			serve(tls.Server(peer, serverTLS))
		})
		conn := tls.Client(client, tlsCfg)
		if err := conn.HandshakeContext(dialCtx); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
	t.Cleanup(func() { rt.CloseIdleConnections(); peers.Wait() })
	return &http.Client{Transport: rt}
}

type wireReplayResponse struct {
	status  int // Zero refuses the stream instead of writing HTTP headers.
	headers []hpack.HeaderField
	body    string
}

func serveDefinitionReplayHTTP2(peer net.Conn, reads chan<- wireDefinitionRead, replay func() bool) {
	first := true
	serveAzureReplayHTTP2(peer, nil, func(req wireDefinitionRead) *wireReplayResponse {
		reads <- req
		if first {
			first = false
			if !replay() {
				return nil
			}
			return &wireReplayResponse{}
		}
		return &wireReplayResponse{status: http.StatusOK, body: emptyMetricDefinitions}
	})
}

func serveAzureReplayHTTP2(peer net.Conn, onHeaders func(), respond func(wireDefinitionRead) *wireReplayResponse) {
	if _, err := io.CopyN(io.Discard, peer, int64(len(http2.ClientPreface))); err != nil {
		return
	}
	incoming := http2.NewFramer(io.Discard, peer)
	incoming.ReadMetaHeaders = hpack.NewDecoder(4096, nil)
	outgoing := http2.NewFramer(peer, nil)
	// net.Pipe has no write buffer. The client may write a SETTINGS ACK
	// while the peer writes REFUSED_STREAM, so read and write independently.
	writes := make(chan func() error, 1)
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		defer peer.Close()
		if outgoing.WriteSettings() != nil {
			return
		}
		for write := range writes {
			if write() != nil {
				return
			}
		}
	}()
	defer func() {
		close(writes)
		_ = peer.Close()
		<-writerDone
	}()
	requests := make(map[uint32]wireDefinitionRead)
	for {
		frame, err := incoming.ReadFrame()
		if err != nil {
			return
		}
		id := frame.Header().StreamID
		req := requests[id]
		var end bool
		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
			if onHeaders != nil {
				onHeaders()
			}
			req = wireDefinitionRead{
				method: frame.PseudoValue("method"), path: frame.PseudoValue("path"), host: frame.PseudoValue("authority"),
			}
			for _, header := range frame.RegularFields() {
				switch header.Name {
				case "authorization":
					req.authorization = header.Value
				case "content-length":
					req.contentLength = header.Value
				case "transfer-encoding":
					req.transferEncoding = header.Value
				}
			}
			end = frame.StreamEnded()
		case *http2.DataFrame:
			req.body += string(frame.Data())
			end = frame.StreamEnded()
		default:
			continue
		}
		if !end {
			requests[id] = req
			continue
		}
		delete(requests, id)
		resp := respond(req)
		if resp == nil {
			return
		}
		if resp.status == 0 {
			writes <- func() error { return outgoing.WriteRSTStream(id, http2.ErrCodeRefusedStream) }
			continue
		}
		writes <- func() error {
			var block bytes.Buffer
			encoder := hpack.NewEncoder(&block)
			_ = encoder.WriteField(hpack.HeaderField{Name: ":status", Value: strconv.Itoa(resp.status)})
			for _, field := range resp.headers {
				_ = encoder.WriteField(field)
			}
			headers := http2.HeadersFrameParam{StreamID: id, BlockFragment: block.Bytes(), EndHeaders: true}
			if err := outgoing.WriteHeaders(headers); err != nil {
				return err
			}
			return outgoing.WriteData(id, true, []byte(resp.body))
		}
	}
}
