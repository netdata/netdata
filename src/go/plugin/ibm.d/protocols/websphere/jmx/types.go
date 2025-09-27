package jmx

import (
	"context"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/jmxbridge"
)

const (
	protocolVersion = "1.0"
)

// Config controls the behaviour of the WebSphere JMX protocol client.
type Config struct {
	JMXURL       string
	JMXUsername  string
	JMXPassword  string
	JMXClasspath string
	JavaExecPath string

	InitTimeout    time.Duration
	CommandTimeout time.Duration
	ShutdownDelay  time.Duration
}

// Option customises a Client instance.
type Option func(*Client)

type bridgeClient interface {
	Start(ctx context.Context, initCmd jmxbridge.Command) error
	Send(ctx context.Context, cmd jmxbridge.Command) (*jmxbridge.Response, error)
	Shutdown()
}

// Client orchestrates helper lifecycle and exposes typed fetch helpers.
type Client struct {
	cfg    Config
	logger jmxbridge.Logger

	jarData []byte
	jarName string

	bridge bridgeClient

	started bool
	mu      sync.Mutex
}

// WithBridge injects a pre-configured bridge client (useful for tests).
func WithBridge(bridge bridgeClient) Option {
	return func(c *Client) {
		c.bridge = bridge
	}
}

// WithJarOverride replaces the embedded helper JAR (testing only).
func WithJarOverride(data []byte, name string) Option {
	return func(c *Client) {
		c.jarData = data
		if name != "" {
			c.jarName = name
		}
	}
}

// JVMStats captures data returned by the helper for the JVM domain.
type JVMStats struct {
	Heap struct {
		Used      float64
		Committed float64
		Max       float64
	}
	NonHeap struct {
		Used      float64
		Committed float64
	}
	GC struct {
		Count float64
		Time  float64
	}
	Threads struct {
		Count   float64
		Daemon  float64
		Peak    float64
		Started float64
	}
	Classes struct {
		Loaded   float64
		Unloaded float64
	}
	CPU struct {
		ProcessUsage float64
	}
	Uptime float64
}

// ThreadPool describes a single thread pool snapshot.
type ThreadPool struct {
	Name            string
	PoolSize        float64
	ActiveCount     float64
	MaximumPoolSize float64
}

// JDBCPool describes a JDBC connection pool snapshot.
type JDBCPool struct {
	Name                    string
	PoolSize                float64
	NumConnectionsUsed      float64
	NumConnectionsFree      float64
	AvgWaitTime             float64
	AvgInUseTime            float64
	NumConnectionsCreated   float64
	NumConnectionsDestroyed float64
	WaitingThreadCount      float64
}

// JCAPool describes a JCA connection pool snapshot.
type JCAPool struct {
	Name                    string
	PoolSize                float64
	NumConnectionsUsed      float64
	NumConnectionsFree      float64
	AvgWaitTime             float64
	AvgInUseTime            float64
	NumConnectionsCreated   float64
	NumConnectionsDestroyed float64
	WaitingThreadCount      float64
}

// JMSDestination describes a JMS destination snapshot.
type JMSDestination struct {
	Name                 string
	Type                 string
	MessagesCurrentCount float64
	MessagesPendingCount float64
	MessagesAddedCount   float64
	ConsumerCount        float64
}

// ApplicationMetric captures web application statistics.
type ApplicationMetric struct {
	Name                   string
	Module                 string
	Requests               float64
	ResponseTime           float64
	ActiveSessions         float64
	LiveSessions           float64
	SessionCreates         float64
	SessionInvalidates     float64
	TransactionsCommitted  float64
	TransactionsRolledback float64
}
