package cgroups_snapshot

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

func snapshotDispatch(handler Handler) raw.DispatchHandler {
	return raw.SnapshotDispatch(handler.Handle, handler.SnapshotMaxItems)
}

// Client is the public L2 client context for the cgroups-snapshot service.
type Client struct {
	inner *raw.Client
}

// NewClient creates a new client context. Does NOT connect.
func NewClient(runDir, serviceName string, config ClientConfig) *Client {
	inner := raw.NewSnapshotClient(runDir, serviceName, clientConfigToTransport(config))
	inner.SetCallTimeout(transportconfig.TypedConfig(config).CallTimeoutMs)
	return &Client{inner: inner}
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

// SetCallTimeout sets the context-level default timeout for blocking calls.
func (c *Client) SetCallTimeout(timeoutMs uint32) {
	c.inner.SetCallTimeout(timeoutMs)
}

// Abort unblocks an in-flight synchronous call.
func (c *Client) Abort() {
	c.inner.Abort()
}

// ClearAbort clears a previous abort request so the client can be reused.
func (c *Client) ClearAbort() {
	c.inner.ClearAbort()
}

// CallSnapshot performs a blocking typed cgroups snapshot call.
func (c *Client) CallSnapshot() (*protocol.CgroupsResponseView, error) {
	return c.CallSnapshotWithTimeout(0)
}

// CallSnapshotWithTimeout performs a blocking typed call with an explicit
// timeout. A zero timeout uses the client's context-level default.
func (c *Client) CallSnapshotWithTimeout(timeoutMs uint32) (*protocol.CgroupsResponseView, error) {
	return c.inner.CallSnapshotWithTimeout(timeoutMs)
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
