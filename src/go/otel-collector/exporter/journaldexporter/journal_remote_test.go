package journaldexporter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"
)

func TestRemoteJournalClient_sendMessage(t *testing.T) {
	tests := map[string]struct {
		setupServer   func(*testing.T) (*httptest.Server, *chunkWriterHandler)
		setupClient   func(*testing.T, string) *remoteJournalClient
		setupContext  func() (context.Context, context.CancelFunc)
		messages      [][]byte
		waitBetween   time.Duration
		waitAfter     time.Duration
		validateErr   func(*testing.T, error)
		validateData  func(*testing.T, *chunkWriterHandler, [][]byte)
		shutdownFirst bool
	}{
		"successful message delivery": {
			setupServer: prepareTestServer,
			setupClient: prepareTestRemoteJournalClient,
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			messages:  [][]byte{[]byte("test message data")},
			waitAfter: 100 * time.Millisecond,
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateData: func(t *testing.T, handler *chunkWriterHandler, messages [][]byte) {
				assert.Equal(t, messages[0], handler.getReceivedData())
			},
		},
		"send empty message": {
			setupServer: prepareTestServer,
			setupClient: prepareTestRemoteJournalClient,
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			messages:  [][]byte{{}},
			waitAfter: 100 * time.Millisecond,
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateData: func(t *testing.T, handler *chunkWriterHandler, messages [][]byte) {
				assert.Empty(t, handler.getReceivedData())
			},
		},
		"send message with cancelled context": {
			setupServer: prepareTestServer,
			setupClient: prepareTestRemoteJournalClient,
			setupContext: func() (context.Context, context.CancelFunc) {
				ctx, cancel := context.WithCancel(context.Background())
				cancel() // Cancel immediately
				return ctx, cancel
			},
			messages:  [][]byte{[]byte("test message that should not be sent")},
			waitAfter: 100 * time.Millisecond,
			validateErr: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.True(t, errors.Is(err, context.Canceled))
			},
		},
		"attempt to send to non-existent server": {
			setupServer: nil, // No server
			setupClient: func(t *testing.T, _ string) *remoteJournalClient {
				return prepareTestRemoteJournalClient(t, "http://non-existent-server:12345")
			},
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			messages:    [][]byte{[]byte("test message to non-existent server")},
			waitBetween: 100 * time.Millisecond,
			validateErr: func(t *testing.T, err error) {
				assert.Error(t, err)
				//assert.Contains(t, err.Error(), "connection unavailable")
			},
		},
		"send after client is shut down": {
			setupServer: prepareTestServer,
			setupClient: prepareTestRemoteJournalClient,
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			messages:      [][]byte{[]byte("test message after shutdown")},
			shutdownFirst: true,
			validateErr: func(t *testing.T, err error) {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "client is shut down")
			},
		},
		// TODO: fix
		//"server responds with error status": {
		//	setupServer: func(t *testing.T) (*httptest.Server, *chunkWriterHandler) {
		//		handler := &chunkWriterHandler{
		//			statusCode: http.StatusInternalServerError,
		//		}
		//		server := httptest.NewServer(handler)
		//		t.Cleanup(func() {
		//			server.Close()
		//		})
		//		return server, handler
		//	},
		//	setupClient: prepareTestRemoteJournalClient,
		//	setupContext: func() (context.Context, context.CancelFunc) {
		//		return context.Background(), func() {}
		//	},
		//	messages: [][]byte{
		//		[]byte("test message expecting error"),
		//		[]byte("second message after error"),
		//	},
		//	waitBetween: 300 * time.Millisecond, // Longer wait to ensure background error is processed
		//	waitAfter:   100 * time.Millisecond,
		//	validateErr: func(t *testing.T, err error) {
		//		// The first message might succeed because the error only occurs after the server responds
		//		// But the second message should fail once the background goroutine sets reqErr
		//		require.Error(t, err)
		//		// If we get an error about pipe being closed or connection unavailable, that's valid
		//		valid := strings.Contains(err.Error(), "connection unavailable") ||
		//			strings.Contains(err.Error(), "failed to write message") ||
		//			strings.Contains(err.Error(), "pipe is closed")
		//		assert.True(t, valid, "Expected error to indicate connection problem, got: %v", err)
		//	},
		//},
		// TODO: fix
		//"server closes connection mid-stream": {
		//	setupServer: func(t *testing.T) (*httptest.Server, *chunkWriterHandler) {
		//		handler := &chunkWriterHandler{
		//			statusCode: http.StatusOK,
		//			closeEarly: true,
		//		}
		//		server := httptest.NewServer(handler)
		//		t.Cleanup(func() {
		//			server.Close()
		//		})
		//		return server, handler
		//	},
		//	setupClient: prepareTestRemoteJournalClient,
		//	setupContext: func() (context.Context, context.CancelFunc) {
		//		return context.Background(), func() {}
		//	},
		//	messages:    [][]byte{[]byte("first message that will cause server to close"), []byte("second message after closure")},
		//	waitBetween: 200 * time.Millisecond,
		//	validateErr: func(t *testing.T, err error) {
		//		assert.Error(t, err)
		//		assert.Contains(t, err.Error(), "connection unavailable")
		//	},
		//},
		"sending multiple messages": {
			setupServer: prepareTestServer,
			setupClient: prepareTestRemoteJournalClient,
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			messages: [][]byte{
				[]byte("first message"),
				[]byte("second message"),
				[]byte("third message"),
			},
			waitAfter: 100 * time.Millisecond,
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateData: func(t *testing.T, handler *chunkWriterHandler, messages [][]byte) {
				expected := append(append(messages[0], messages[1]...), messages[2]...)
				assert.Equal(t, expected, handler.getReceivedData())
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var server *httptest.Server
			var handler *chunkWriterHandler
			var client *remoteJournalClient

			if tc.setupServer != nil {
				server, handler = tc.setupServer(t)
				client = tc.setupClient(t, server.URL)
			} else {
				client = tc.setupClient(t, "")
			}
			defer func() { _ = client.shutdown(context.Background()) }()

			ctx, cancel := tc.setupContext()
			defer cancel()

			if tc.shutdownFirst {
				err := client.shutdown(context.Background())
				assert.NoError(t, err)
			}

			var lastErr error
			for i, msg := range tc.messages {
				lastErr = client.sendMessage(ctx, msg)

				// For multi-message tests, we only validate error on the last message
				if i < len(tc.messages)-1 && tc.waitBetween > 0 {
					time.Sleep(tc.waitBetween)
				}
			}

			if tc.waitAfter > 0 {
				time.Sleep(tc.waitAfter)
			}

			if tc.validateErr != nil {
				tc.validateErr(t, lastErr)
			}

			if tc.validateData != nil && handler != nil {
				tc.validateData(t, handler, tc.messages)
			}
		})
	}
}

func TestRemoteJournalClient_shutdown(t *testing.T) {
	tests := map[string]struct {
		setupServer      func(*testing.T) (*httptest.Server, *chunkWriterHandler)
		sendMessages     func(*testing.T, context.Context, *remoteJournalClient)
		setupContext     func() (context.Context, context.CancelFunc)
		validateErr      func(*testing.T, error)
		validateShutdown func(*testing.T, *remoteJournalClient, *chunkWriterHandler)
		doubleShutdown   bool
	}{
		"normal shutdown": {
			setupServer: prepareTestServer,
			sendMessages: func(t *testing.T, ctx context.Context, client *remoteJournalClient) {
				msg := []byte("test message before shutdown")
				err := client.sendMessage(ctx, msg)
				assert.NoError(t, err)
			},
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateShutdown: func(t *testing.T, client *remoteJournalClient, handler *chunkWriterHandler) {
				select {
				case <-client.done:
					// Expected - channel should be closed
				default:
					t.Error("Expected done channel to be closed after shutdown")
				}

				err := client.sendMessage(context.Background(), []byte("test message after shutdown"))
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "client is shut down")
			},
		},
		// TODO: fix
		//"shutdown with timeout": {
		//	setupServer: func(t *testing.T) (*httptest.Server, *chunkWriterHandler) {
		//		handler := &chunkWriterHandler{
		//			statusCode: http.StatusOK,
		//			delay:      500 * time.Millisecond, // Delay each read to simulate slow processing
		//		}
		//		server := httptest.NewServer(handler)
		//		t.Cleanup(func() {
		//			server.Close()
		//		})
		//		return server, handler
		//	},
		//	sendMessages: func(t *testing.T, ctx context.Context, client *remoteJournalClient) {
		//		// Send a large message to ensure the background task will be busy
		//		msg := []byte(strings.Repeat("test message with shutdown timeout ", 1000))
		//		err := client.sendMessage(ctx, msg)
		//		assert.NoError(t, err)
		//	},
		//	setupContext: func() (context.Context, context.CancelFunc) {
		//		return context.WithTimeout(context.Background(), 100*time.Millisecond)
		//	},
		//	validateErr: func(t *testing.T, err error) {
		//		assert.Error(t, err)
		//		assert.True(t, errors.Is(err, context.DeadlineExceeded))
		//	},
		//	validateShutdown: func(t *testing.T, client *remoteJournalClient, handler *chunkWriterHandler) {
		//		select {
		//		case <-client.done:
		//			// Expected - channel should be closed
		//		default:
		//			t.Error("Expected done channel to be closed after shutdown timeout")
		//		}
		//	},
		//},
		"double shutdown": {
			setupServer: prepareTestServer,
			sendMessages: func(t *testing.T, ctx context.Context, client *remoteJournalClient) {
				// No messages sent
			},
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			doubleShutdown: true,
		},
		"shutdown with active connections": {
			setupServer: prepareTestServer,
			sendMessages: func(t *testing.T, ctx context.Context, client *remoteJournalClient) {
				// Send multiple messages to ensure there's an active connection
				for i := 0; i < 5; i++ {
					msg := []byte(fmt.Sprintf("test message %d before shutdown", i))
					err := client.sendMessage(ctx, msg)
					assert.NoError(t, err)
				}
				// Allow some time for the background goroutine to process
				time.Sleep(50 * time.Millisecond)
			},
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateShutdown: func(t *testing.T, client *remoteJournalClient, handler *chunkWriterHandler) {
				// Verify at least some data was received before shutdown
				receivedData := handler.getReceivedData()
				assert.NotEmpty(t, receivedData)
				assert.Contains(t, string(receivedData), "test message")
			},
		},
		"shutdown without active connections": {
			setupServer: prepareTestServer,
			sendMessages: func(t *testing.T, ctx context.Context, client *remoteJournalClient) {
				// No messages sent
			},
			setupContext: func() (context.Context, context.CancelFunc) {
				return context.Background(), func() {}
			},
			validateErr: func(t *testing.T, err error) {
				assert.NoError(t, err)
			},
			validateShutdown: func(t *testing.T, client *remoteJournalClient, handler *chunkWriterHandler) {
				select {
				case <-client.done:
					// Expected - channel should be closed
				default:
					t.Error("Expected done channel to be closed after shutdown")
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			server, handler := tc.setupServer(t)

			client := prepareTestRemoteJournalClient(t, server.URL)

			// Setup context for messages and initial operations
			msgCtx, msgCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer msgCancel()

			tc.sendMessages(t, msgCtx, client)

			shutdownCtx, shutdownCancel := tc.setupContext()
			defer shutdownCancel()

			err := client.shutdown(shutdownCtx)

			if tc.doubleShutdown {
				secondErr := client.shutdown(context.Background())
				assert.NoError(t, secondErr, "Second shutdown should not error")
			}

			if tc.validateErr != nil {
				tc.validateErr(t, err)
			}

			if tc.validateShutdown != nil {
				tc.validateShutdown(t, client, handler)
			}
		})
	}
}

type chunkWriterHandler struct {
	receivedData   []byte
	mu             sync.Mutex
	delay          time.Duration
	statusCode     int
	errorAfterRead bool
	closeEarly     bool
}

func (h *chunkWriterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.receivedData = make([]byte, 0)
	h.mu.Unlock()

	contentType := r.Header.Get("Content-Type")
	if contentType != "application/vnd.fdo.journal" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Invalid content type"))
		return
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.WriteHeader(h.statusCode)

	buf := make([]byte, 1024)
	readCount := 0

	readDone := make(chan struct{})
	go func() {
		defer close(readDone)

		for {
			if h.delay > 0 {
				time.Sleep(h.delay)
			}
			n, err := r.Body.Read(buf)
			if n > 0 {
				h.mu.Lock()
				h.receivedData = append(h.receivedData, buf[:n]...)
				h.mu.Unlock()
				readCount++
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return
			}
			if h.errorAfterRead && readCount >= 1 {
				if flusher, ok := w.(http.Flusher); ok {
					flusher.Flush()
				}
				return
			}
			if h.closeEarly && readCount >= 1 {
				return
			}
			if h.statusCode < 200 || h.statusCode >= 300 {
				return
			}
		}
	}()

	<-readDone
}

func (h *chunkWriterHandler) getReceivedData() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.receivedData
}

func prepareTestServer(t *testing.T) (*httptest.Server, *chunkWriterHandler) {
	handler := &chunkWriterHandler{
		statusCode: http.StatusOK,
	}
	server := httptest.NewServer(handler)
	t.Cleanup(func() {
		server.Close()
	})
	return server, handler
}

func prepareTestRemoteJournalClient(t *testing.T, serverURL string) *remoteJournalClient {
	cfg := &Config{
		URL:     serverURL,
		Timeout: 1 * time.Second,
	}

	client, err := newRemoteJournalClient(cfg)
	require.NoError(t, err)
	require.NotNil(t, client)

	client.log = zaptest.NewLogger(t)
	return client
}
