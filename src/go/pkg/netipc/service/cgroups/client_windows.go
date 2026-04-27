//go:build windows

package cgroups

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
	windows "github.com/netdata/netdata/go/plugins/pkg/netipc/transport/windows"
)

func snapshotDispatch(handler Handler) raw.DispatchHandler {
	return raw.SnapshotDispatch(handler.Handle, handler.SnapshotMaxItems)
}

func clientConfigToTransport(config ClientConfig) windows.ClientConfig {
	return windows.ClientConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   config.MaxRequestBatchItems,
		AuthToken:               config.AuthToken,
	}
}

func serverConfigToTransport(config ServerConfig) windows.ServerConfig {
	return windows.ServerConfig{
		SupportedProfiles:       config.SupportedProfiles,
		PreferredProfiles:       config.PreferredProfiles,
		MaxRequestBatchItems:    config.MaxRequestBatchItems,
		MaxResponsePayloadBytes: config.MaxResponsePayloadBytes,
		MaxResponseBatchItems:   config.MaxRequestBatchItems,
		AuthToken:               config.AuthToken,
	}
}

// Client is the public L2 client context for the cgroups-snapshot service.
type Client struct {
	inner *raw.Client
}

// NewClient creates a new client context. Does NOT connect.
func NewClient(runDir, serviceName string, config ClientConfig) *Client {
	return &Client{inner: raw.NewSnapshotClient(runDir, serviceName, clientConfigToTransport(config))}
}

// Refresh attempts connect if DISCONNECTED/NOT_FOUND, reconnect if BROKEN.
func (c *Client) Refresh() bool {
	return c.inner.Refresh()
}

// Ready returns true only if the client is in the READY state.
func (c *Client) Ready() bool {
	return c.inner.Ready()
}

// Status returns a diagnostic counters snapshot.
func (c *Client) Status() ClientStatus {
	return c.inner.Status()
}

// CallSnapshot performs a blocking typed cgroups snapshot call.
func (c *Client) CallSnapshot() (*protocol.CgroupsResponseView, error) {
	return c.inner.CallSnapshot()
}

// Close tears down the connection and releases resources.
func (c *Client) Close() {
	c.inner.Close()
}

// Server is the public managed server for the cgroups-snapshot service kind.
type Server struct {
	inner *raw.Server
}

// NewServer creates a new managed server.
func NewServer(runDir, serviceName string, config ServerConfig, handler Handler) *Server {
	return &Server{
		inner: raw.NewServer(
			runDir,
			serviceName,
			serverConfigToTransport(config),
			protocol.MethodCgroupsSnapshot,
			snapshotDispatch(handler),
		),
	}
}

// NewServerWithWorkers creates a server with an explicit worker count limit.
func NewServerWithWorkers(runDir, serviceName string, config ServerConfig,
	handler Handler, workerCount int) *Server {
	return &Server{
		inner: raw.NewServerWithWorkers(
			runDir,
			serviceName,
			serverConfigToTransport(config),
			protocol.MethodCgroupsSnapshot,
			snapshotDispatch(handler),
			workerCount,
		),
	}
}

// Run starts the acceptor loop. Blocking.
func (s *Server) Run() error {
	return s.inner.Run()
}

// Stop signals the server to stop.
func (s *Server) Stop() {
	s.inner.Stop()
}
