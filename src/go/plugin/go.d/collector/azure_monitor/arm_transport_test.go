// SPDX-License-Identifier: GPL-3.0-or-later

package azure_monitor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
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
	bodyBytes                         int
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
	var peers sync.WaitGroup
	dial := func(serve func(net.Conn, chan<- wireDefinitionRead, func() bool)) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		client, peer := net.Pipe()
		deadline, _ := ctx.Deadline()
		_ = peer.SetDeadline(deadline)
		peers.Go(func() {
			defer peer.Close()
			serve(peer, reads, replay)
		})
		return client, nil
	}
	if protocol == "HTTP/2" {
		rt := &http2.Transport{DialTLSContext: func(context.Context, string, string, *tls.Config) (net.Conn, error) {
			return dial(serveDefinitionReplayHTTP2)
		}}
		t.Cleanup(func() { rt.CloseIdleConnections(); peers.Wait() })
		return &http.Client{Transport: rt}
	}
	rt := &http.Transport{DialTLSContext: func(context.Context, string, string) (net.Conn, error) {
		return dial(serveDefinitionReplayHTTP1)
	}}
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
		body, err := io.Copy(io.Discard, req.Body)
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
			bodyBytes:        int(body),
		}
		if warmed {
			replay() // EOF after the server has read the entire request.
			return
		}
		_, _ = fmt.Fprintf(peer, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n%s", len(emptyMetricDefinitions), emptyMetricDefinitions)
	}
}

func serveDefinitionReplayHTTP2(peer net.Conn, reads chan<- wireDefinitionRead, replay func() bool) {
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
	var req wireDefinitionRead
	for {
		frame, err := incoming.ReadFrame()
		if err != nil {
			return
		}
		var end bool
		switch frame := frame.(type) {
		case *http2.MetaHeadersFrame:
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
			req.bodyBytes += len(frame.Data())
			end = frame.StreamEnded()
		}
		if !end {
			continue
		}
		reads <- req
		id := frame.Header().StreamID
		if id == 1 {
			if !replay() {
				return
			}
			writes <- func() error { return outgoing.WriteRSTStream(id, http2.ErrCodeRefusedStream) }
			continue
		}
		writes <- func() error {
			var block bytes.Buffer
			_ = hpack.NewEncoder(&block).WriteField(hpack.HeaderField{Name: ":status", Value: "200"})
			headers := http2.HeadersFrameParam{StreamID: id, BlockFragment: block.Bytes(), EndHeaders: true}
			if err := outgoing.WriteHeaders(headers); err != nil {
				return err
			}
			return outgoing.WriteData(id, true, []byte(emptyMetricDefinitions))
		}
	}
}
