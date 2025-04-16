package journaldexporter

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

func TestRemoteJournalClient_sendMessage(t *testing.T) {
	tests := map[string]struct {
		prepare  func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler)
		messages []string
		wantErr  bool
	}{
		"successful message delivery": {
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				return prepareTestServer(t)
			},
			messages: []string{
				`{"message":"hello world1"}`,
				`{"message":"hello world2"}`,
				`{"message":"hello world3"}`,
			},
		},
		"error message delivery": {
			wantErr: true,
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				srv, h := prepareTestServer(t)
				h.errorAfterRead = true
				return srv, h
			},
			messages: []string{
				`{"message":"hello world1"}`,
				`{"message":"hello world2"}`,
				`{"message":"hello world3"}`,
			},
		},
		"connection abrupt close": {
			wantErr: true,
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				srv, h := prepareTestServer(t)
				h.closeEarly = true
				return srv, h
			},
			messages: []string{
				`{"message":"hello world1"}`,
				`{"message":"hello world2"}`,
			},
		},
		"attempt to send to non-existent server": {
			wantErr: true,
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
				srv.Close()

				return srv, nil
			},
			messages: []string{
				`{"message":"hello world1"}`,
			},
		},
		"empty message": {
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				return prepareTestServer(t)
			},
			messages: []string{
				``,
			},
		},
		"large message": {
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				return prepareTestServer(t)
			},
			messages: []string{
				strings.Repeat(`{"message":"large payload test"}`, 1000),
			},
		},
		"slow server response": {
			prepare: func(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
				srv, h := prepareTestServer(t)
				h.readDelay = 200 * time.Millisecond
				return srv, h
			},
			messages: []string{
				`{"message":"hello world1"}`,
				`{"message":"hello world2"}`,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			srv, h := test.prepare(t)
			if srv != nil {
				defer srv.Close()
			}

			jc := prepareRemoteJournalClient(t, srv.URL)

			var wg sync.WaitGroup
			sendErrCh := make(chan error, 1)
			recvErrCh := make(chan error, 1)

			// Skip receive handling for non-existent server and empty message cases
			skipReceive := h == nil || len(test.messages) == 0 ||
				(len(test.messages) == 1 && test.messages[0] == "")

			wg.Add(1)
			go func() {
				defer wg.Done()
				var err error
				for _, msg := range test.messages {
					err = errors.Join(err, jc.sendMessage(context.Background(), []byte(msg)))
					// Brief pause to allow processing
					time.Sleep(50 * time.Millisecond)
				}
				sendErrCh <- err
			}()

			if skipReceive {
				// For non-server or empty message tests, we don't expect to receive anything
				recvErrCh <- nil
			} else {
				wg.Add(1)
				go func() {
					defer wg.Done()

					want := strings.Join(test.messages, "")
					var buf bytes.Buffer
					timeout := 5 * time.Second

					// For large payloads, extend timeout
					if len(want) > 10000 {
						timeout = 10 * time.Second
					}

					timeoutCh := time.After(timeout)

					for {
						select {
						case msg, ok := <-h.recDataCh:
							if !ok {
								// If we're expecting an error due to connection close,
								// this is normal and not a test failure
								if test.wantErr && buf.Len() > 0 {
									recvErrCh <- nil
								} else if buf.String() == want {
									recvErrCh <- nil
								} else if buf.Len() > 0 {
									recvErrCh <- fmt.Errorf("received incomplete data: got %q, want %q",
										buf.String(), want)
								} else {
									recvErrCh <- errors.New("channel closed without receiving data")
								}
								return
							}
							_, _ = buf.Write(bytes.Clone(msg))
							if buf.String() == want {
								recvErrCh <- nil
								return
							}
						case <-timeoutCh:
							recvErrCh <- fmt.Errorf("timeout waiting for messages after %v", timeout)
							return
						}
					}
				}()
			}

			done := make(chan struct{})
			go func() {
				defer close(done)
				wg.Wait()
			}()

			select {
			case <-done:
			case <-time.After(10 * time.Second):
				_ = jc.shutdown(context.Background())
				t.Fatal("timed out waiting for test completion")
			}

			shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := jc.shutdown(shutdownCtx)
			if err != nil {
				t.Logf("Shutdown error (non-fatal): %v", err)
			}

			// Give the test a moment to complete any pending operations
			time.Sleep(100 * time.Millisecond)

			var sendErr, recvErr error
			select {
			case sendErr = <-sendErrCh:
			default:
				sendErr = errors.New("send goroutine did not complete")
			}

			if !skipReceive {
				select {
				case recvErr = <-recvErrCh:
				default:
					recvErr = errors.New("receive goroutine did not complete")
				}
			}

			combinedErr := errors.Join(sendErr, recvErr)

			if test.wantErr {
				if name == "empty message" {
					// Empty message should not cause an error
					assert.NoError(t, combinedErr, "empty message should not cause error")
				} else {
					assert.Error(t, combinedErr, "expected an error but got none")
				}
			} else {
				assert.NoError(t, combinedErr, "unexpected error")
			}
		})
	}
}

func TestRemoteJournalClient_ConcurrentMessages(t *testing.T) {
	srv, h := prepareTestServer(t)
	defer srv.Close()

	jc := prepareRemoteJournalClient(t, srv.URL)

	const numMessages = 10
	const numGoroutines = 5

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				msg := fmt.Sprintf(`{"goroutine":%d,"message":%d}`, id, j)
				err := jc.sendMessage(context.Background(), []byte(msg))
				if err != nil {
					t.Errorf("Error sending message from goroutine %d: %v", id, err)
					return
				}
				// Small delay to interleave messages
				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	received := make([]string, 0, numMessages*numGoroutines)
	receiveDone := make(chan struct{})

	go func() {
		defer close(receiveDone)
		for {
			select {
			case msg, ok := <-h.recDataCh:
				if !ok {
					return
				}
				received = append(received, string(msg))
			case <-time.After(5 * time.Second):
				t.Error("Timeout waiting for messages")
				return
			}
		}
	}()

	wg.Wait()

	err := jc.shutdown(context.Background())
	require.NoError(t, err)

	select {
	case <-receiveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for receiver to complete")
	}

	t.Logf("Received %d message chunks", len(received))
	fullMessage := strings.Join(received, "")

	messageCount := strings.Count(fullMessage, `{"goroutine":`)
	assert.GreaterOrEqual(t, messageCount, numMessages*numGoroutines,
		"Did not receive all expected messages")
}

func TestRemoteJournalClient_ContextCancellation(t *testing.T) {
	srv, _ := prepareTestServer(t)
	defer srv.Close()

	jc := prepareRemoteJournalClient(t, srv.URL)

	ctx, cancel := context.WithCancel(context.Background())

	cancel()

	err := jc.sendMessage(ctx, []byte(`{"message":"should not be sent"}`))
	assert.Error(t, err, "Expected error due to cancelled context")
	assert.ErrorIs(t, err, context.Canceled, "Error should be context.Canceled")

	_ = jc.shutdown(context.Background())
}

func TestRemoteJournalClient_SendAfterShutdown(t *testing.T) {
	srv, _ := prepareTestServer(t)
	defer srv.Close()

	jc := prepareRemoteJournalClient(t, srv.URL)

	err := jc.shutdown(context.Background())
	require.NoError(t, err, "Shutdown should succeed")

	err = jc.sendMessage(context.Background(), []byte(`{"message":"after shutdown"}`))
	assert.Error(t, err, "Expected error when sending to shut down client")
	assert.Contains(t, err.Error(), "client is shut down",
		"Error should indicate client is shut down")
}

type chunkReaderTestHandler struct {
	log            *zap.Logger
	recDataCh      chan []byte
	readDelay      time.Duration // Delay between reads (simulates slow processing)
	errorAfterRead bool          // If true, send an error status *after* reading at least one chunk
	closeEarly     bool          // If true, simulate an abrupt connection close *after* reading at least one chunk
}

func newChunkReaderTestHandler(t *testing.T) *chunkReaderTestHandler {
	return &chunkReaderTestHandler{
		log:       zaptest.NewLogger(t),
		recDataCh: make(chan []byte),
	}
}

func (h *chunkReaderTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		close(h.recDataCh)
	}()

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/vnd.fdo.journal" {
		h.log.Error("Received invalid content type", zap.String("content-type", contentType))
		http.Error(w, "Invalid content type", http.StatusBadRequest)
		return
	}

	buf := make([]byte, 1024)

	for {
		if h.readDelay > 0 {
			time.Sleep(h.readDelay)
		}

		n, err := r.Body.Read(buf)
		if err != nil {
			if err == io.EOF {
				h.log.Info("Finished reading request body (EOF)")
				break
			}
			h.log.Error("Error reading request body", zap.Error(err))
			http.Error(w, fmt.Sprintf("Error reading request body: %v", err), http.StatusInternalServerError)
			return
		}

		if h.errorAfterRead {
			h.log.Info("Simulating error response after read")
			http.Error(w, "Simulated processing error after read", http.StatusInternalServerError)
			return
		}

		if h.closeEarly {
			h.log.Info("Simulating connection abrupt close")
			hj, ok := w.(http.Hijacker)
			if !ok {
				h.log.Warn("Cannot simulate close early: Hijacking not supported")
				http.Error(w, "Cannot simulate early close (hijacking not supported)", http.StatusInternalServerError)
				return
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
			fmt.Println("Connection closed abruptly.")
			return
		}

		select {
		case h.recDataCh <- bytes.Clone(buf[:n]):
		case <-time.After(time.Second * 5):
			http.Error(w, "Sending read data back timed out", http.StatusLocked)
			return
		}
	}

	_, _ = io.Copy(io.Discard, r.Body)
	_ = r.Body.Close()
	w.WriteHeader(http.StatusOK)
}

func prepareTestServer(t *testing.T) (*httptest.Server, *chunkReaderTestHandler) {
	handler := newChunkReaderTestHandler(t)

	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})
	return server, handler
}

func prepareRemoteJournalClient(t *testing.T, serverURL string) *remoteJournalClient {
	cfg := &Config{
		URL:     serverURL,
		Timeout: 3 * time.Second,
	}

	client, err := newRemoteJournalClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	client.log = zaptest.NewLogger(t)

	return client
}
