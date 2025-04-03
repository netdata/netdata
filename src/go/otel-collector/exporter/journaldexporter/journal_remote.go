package journaldexporter

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"

	"go.uber.org/zap"
)

func newRemoteJournalClient(cfg *Config) (*remoteJournalClient, error) {
	client, err := newHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create remote journal http client: %v", err)
	}
	return &remoteJournalClient{
		remoteURL:  cfg.URL,
		httpClient: client,
		done:       make(chan struct{}),
	}, nil
}

type remoteJournalClient struct {
	log *zap.Logger

	remoteURL  string
	httpClient *http.Client

	mu        sync.Mutex         // Protects access to writer, error info, cancel func
	w         io.WriteCloser     // The pipe writer for the current active upload stream
	reqErr    error              // Stores the error from the last background upload attempt
	reqCancel context.CancelFunc // Function to cancel the current background HTTP request

	done chan struct{}  // Closed when the httpClient is shutting down
	wg   sync.WaitGroup // Waits for background operations to complete during done
}

func (jc *remoteJournalClient) sendMessage(ctx context.Context, msg []byte) error {
	if len(msg) == 0 {
		return nil
	}

	var currentWriter io.WriteCloser
	var connectErr error

	jc.mu.Lock()

	select {
	case <-ctx.Done():
		jc.mu.Unlock()
		return ctx.Err()
	case <-jc.done:
		jc.mu.Unlock()
		return errors.New("journal: client is shut down")
	default:
	}

	if jc.w == nil {
		jc.log.Info("journal: not connected, attempting to connect...")
		connectErr = jc.connectLocked(ctx)
	}

	if connectErr == nil && jc.w != nil {
		currentWriter = jc.w
	}
	lastReqErr := jc.reqErr
	jc.mu.Unlock()

	if connectErr != nil {
		return fmt.Errorf("journal: connection attempt failed: %w", connectErr)
	}

	if currentWriter == nil {
		errMsg := "journal: connection unavailable"
		if lastReqErr != nil {
			return fmt.Errorf("%s (last background error: %w)", errMsg, lastReqErr)
		}
		return errors.New(errMsg)
	}

	if _, err := currentWriter.Write(msg); err != nil {
		return fmt.Errorf("journal: failed to write message (connection likely closed): %w", err)
	}

	return nil
}

// connectLocked initiates a new upload stream. Must be called with jc.mu held.
func (jc *remoteJournalClient) connectLocked(ctx context.Context) error {
	if jc.w != nil {
		jc.log.Warn("journal: warning - connectLocked called while already connected")
		jc.disconnectLocked()
	}

	if jc.isDone() {
		return errors.New("journal: client is shut down")
	}

	reqCtx, cancel := context.WithCancel(ctx)
	jc.reqCancel = cancel

	pr, pw := io.Pipe()
	jc.w = pw
	jc.reqErr = nil

	jc.wg.Add(1)
	go func() {
		defer jc.wg.Done()

		// blocks until the stream finishes.
		reqErr := jc.doRequest(reqCtx, pr)

		jc.mu.Lock()
		if jc.w == pw {
			jc.reqErr = reqErr
			jc.w = nil
			jc.reqCancel = nil
		}
		jc.mu.Unlock()

		if reqErr != nil {
			if errors.Is(reqErr, context.Canceled) || errors.Is(reqErr, io.ErrClosedPipe) {
				jc.log.Info("journal: background upload finished successfully.")
			} else {
				jc.log.Error("journal: background upload failed with error", zap.Error(reqErr))
			}
		}
	}()

	jc.log.Info("journal: connection attempt initiated (running in background)")

	return nil
}

func (jc *remoteJournalClient) doRequest(ctx context.Context, pr *io.PipeReader) (err error) {
	defer func() { _ = pr.CloseWithError(err) }()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, jc.remoteURL, pr)
	if err != nil {
		return fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.fdo.journal")

	// This call blocks until the request body (pr) is closed, the context is canceled,
	// the server responds AND closes the connection, or a connection error occurs.
	resp, err := jc.httpClient.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			jc.log.Info("journal: request cancelled via context.")
		} else {
			jc.log.Warn(fmt.Sprintf("journal: http client error: %v", err))
		}
		return err
	}

	defer closeBody(resp)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

func (jc *remoteJournalClient) shutdown(ctx context.Context) error {
	jc.mu.Lock()
	if jc.isDone() {
		jc.mu.Unlock()
		jc.log.Warn("journal: client is shut down")
		return nil
	}

	close(jc.done)
	jc.log.Info("journal: shutdown initiated.")
	jc.disconnectLocked()

	jc.mu.Unlock()

	done := make(chan struct{})
	go func() {
		jc.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		jc.log.Info("journal: all background tasks finished.")
		return nil
	case <-ctx.Done():
		jc.log.Info("journal: shutdown timed out waiting for background tasks.")
		return ctx.Err()
	}
}

// disconnectLocked cancels the current request and closes the writer pipe. Must be called with jc.mu held.
func (jc *remoteJournalClient) disconnectLocked() {
	if jc.w != nil {
		_ = jc.w.Close()
		jc.w = nil
	}
	if jc.reqCancel != nil {
		jc.reqCancel()
		jc.reqCancel = nil
	}
}

func (jc *remoteJournalClient) isDone() bool {
	select {
	case <-jc.done:
		return true
	default:
		return false
	}
}

func newHTTPClient(cfg *Config) (*http.Client, error) {
	tlsConfig, err := newTLSConfig(cfg)
	if err != nil {
		return nil, err
	}

	d := &net.Dialer{Timeout: cfg.Timeout}

	client := http.Client{
		Timeout: cfg.Timeout,
		Transport: &http.Transport{
			TLSClientConfig:     tlsConfig,
			DialContext:         d.DialContext,
			TLSHandshakeTimeout: cfg.Timeout,
		},
	}

	return &client, nil
}

func newTLSConfig(cfg *Config) (*tls.Config, error) {
	if cfg.TLS.SrvCertFile == "" && cfg.TLS.SrvKeyFile == "" && cfg.TLS.TrustedCertFile == "" && !cfg.TLS.InsecureSkipVerify {
		return nil, nil
	}

	var clientCerts []tls.Certificate
	if cfg.TLS.SrvCertFile != "" && cfg.TLS.SrvKeyFile != "" {
		clientCert, err := tls.LoadX509KeyPair(cfg.TLS.SrvCertFile, cfg.TLS.SrvKeyFile)
		if err != nil {
			return nil, fmt.Errorf("error loading tls cert and key files: %s", err)
		}
		clientCerts = append(clientCerts, clientCert)
	}

	tlsConfig := &tls.Config{
		Certificates: clientCerts,
		MinVersion:   tls.VersionTLS12,
	}

	if cfg.TLS.TrustedCertFile != "" {
		caCert, err := os.ReadFile(cfg.TLS.TrustedCertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate file %q: %w", cfg.TLS.TrustedCertFile, err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate from %q", cfg.TLS.TrustedCertFile)
		}
		tlsConfig.RootCAs = caCertPool
		tlsConfig.InsecureSkipVerify = false
	} else if cfg.TLS.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return tlsConfig, nil
}

func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
}
