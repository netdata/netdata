# PCF Protocol Package

This package provides a PCF (Programmable Command Format) protocol implementation for communicating with IBM MQ Queue Managers.

## Features

- Full PCF protocol implementation with CGO bindings to IBM MQ C libraries
- Connection management with automatic reconnection and exponential backoff
- Static data caching (version, edition, platform) refreshed on reconnection
- Support for all major PCF commands (queues, channels, topics, etc.)
- Multi-message response handling for wildcard queries
- Human-readable error messages

## Usage

```go
import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"

// Create client
client := pcf.NewClient(pcf.Config{
    QueueManager: "QM1",
    Channel:      "SYSTEM.DEF.SVRCONN",
    Host:         "localhost",
    Port:         1414,
    User:         "admin",
    Password:     "password",
}, state)

// Connect
if err := client.Connect(); err != nil {
    return err
}
defer client.Disconnect()

// Get version (cached, refreshed on reconnection)
version, edition, err := client.GetVersion()

// Get queue metrics
metrics, err := client.GetQueueMetrics("SYSTEM.DEFAULT.LOCAL.QUEUE")

// Get queue list
queues, err := client.GetQueueList()
```

## Requirements

- IBM MQ Client libraries installed
- CGO enabled
- Linux platform

## Protocol Details

The PCF protocol uses IBM MQ's administrative interface to query queue manager statistics and configuration. Commands are sent to the `SYSTEM.ADMIN.COMMAND.QUEUE` and responses are received on a temporary dynamic queue.

### Connection Lifecycle

1. Connect to queue manager using MQCONNX
2. Open command queue (SYSTEM.ADMIN.COMMAND.QUEUE)
3. Create temporary reply queue
4. Refresh static data (version, platform)
5. Ready for PCF commands

### Error Handling

The protocol automatically detects connection failures and marks itself as disconnected. The framework's backoff mechanism handles reconnection attempts.

### Caching

Static data like version and platform information is cached and only refreshed on reconnection, reducing overhead for frequently accessed information.