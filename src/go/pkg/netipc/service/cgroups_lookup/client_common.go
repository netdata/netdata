package cgroups_lookup

import (
	"github.com/netdata/netdata/go/plugins/pkg/netipc/protocol"
	"github.com/netdata/netdata/go/plugins/pkg/netipc/service/internal/transportconfig"
	raw "github.com/netdata/netdata/go/plugins/pkg/netipc/service/raw"
)

type Client struct {
	inner *raw.Client
}

func NewClient(runDir, serviceName string, config ClientConfig) *Client {
	typedConfig := transportconfig.TypedConfig(config)
	inner := raw.NewCgroupsLookupClient(runDir, serviceName, clientConfigToTransport(config))
	inner.SetCallTimeout(typedConfig.CallTimeoutMs)
	inner.SetLookupLogicalConfig(raw.LookupLogicalConfig{
		MaxItems:         typedConfig.MaxLogicalLookupItems,
		MaxSubcalls:      typedConfig.MaxLogicalLookupSubcalls,
		MaxResponseBytes: typedConfig.MaxLogicalLookupResponseBytes,
	})
	return &Client{inner: inner}
}

func (c *Client) Refresh() bool { return c.inner.Refresh() }
func (c *Client) Ready() bool   { return c.inner.Ready() }
func (c *Client) Status() ClientStatus {
	return c.inner.Status()
}
func (c *Client) SetCallTimeout(timeoutMs uint32) { c.inner.SetCallTimeout(timeoutMs) }
func (c *Client) Abort()                          { c.inner.Abort() }
func (c *Client) ClearAbort()                     { c.inner.ClearAbort() }
func (c *Client) Call(paths [][]byte) (*protocol.CgroupsLookupResponseView, error) {
	return c.CallWithTimeout(paths, 0)
}
func (c *Client) CallWithTimeout(paths [][]byte, timeoutMs uint32) (*protocol.CgroupsLookupResponseView, error) {
	return c.inner.CallCgroupsLookupWithTimeout(paths, timeoutMs)
}
func (c *Client) Close() { c.inner.Close() }

type Server struct {
	inner *raw.Server
}

func NewServer(runDir, serviceName string, config ServerConfig, handler Handler) *Server {
	return &Server{inner: raw.NewServer(runDir, serviceName, serverConfigToTransport(config), protocol.MethodCgroupsLookup, raw.CgroupsLookupDispatch(handler.Handle))}
}

func NewServerWithWorkers(runDir, serviceName string, config ServerConfig, handler Handler, workerCount int) *Server {
	return &Server{inner: raw.NewServerWithWorkers(runDir, serviceName, serverConfigToTransport(config), protocol.MethodCgroupsLookup, raw.CgroupsLookupDispatch(handler.Handle), workerCount)}
}

func (s *Server) Run() error { return s.inner.Run() }
func (s *Server) Stop()      { s.inner.Stop() }
