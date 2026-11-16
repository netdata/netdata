// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/stmcginnis/gofish/schemas"
)

var (
	ErrLogServiceUnavailable = errors.New(
		"selected Redfish log service is no longer advertised; refresh the service choices",
	)
	ErrLogEntriesUnsupported = errors.New("selected Redfish log service does not advertise an Entries collection")
)

// LogService identifies a linked BMC log. No SDK objects or sessions escape a query.
type LogService struct{ URI, Name string }

type LogEntry struct {
	URI, ID, EntryType, Severity, Message, MessageID string
	EventTimestamp, Created                          string
}

type LogResult struct {
	Services []LogService
	Entries  []LogEntry
}

// LogReader opens an independent SDK session for each request. It is safe to use
// concurrently with metric acquisition and with other Function requests.
type LogReader struct{ connection *connection }

func (c *Client) Logs() *LogReader {
	return &LogReader{
		connection: &c.connection,
	}
}

func (r *LogReader) Services(ctx context.Context) ([]LogService, error) {
	var result []LogService
	err := r.query(ctx, func(c *logClient) error {
		services, err := c.services()
		if err != nil {
			return err
		}
		result = logServiceChoices(services)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (r *LogReader) Entries(ctx context.Context, serviceURI string) (LogResult, error) {
	var result LogResult
	err := r.query(ctx, func(c *logClient) error {
		services, err := c.services()
		if err != nil {
			return err
		}
		service, ok := services[serviceURI]
		if !ok {
			return ErrLogServiceUnavailable
		}
		var links struct{ Entries schemas.Link }
		if json.Unmarshal(service.RawData, &links) != nil || links.Entries.String() == "" {
			return ErrLogEntriesUnsupported
		}
		entries, err := service.Entries()
		if err != nil {
			return errors.New(
				"could not read the complete Redfish Entries collection; check BMC availability and read permissions",
			)
		}
		result = LogResult{
			Services: logServiceChoices(services),
			Entries:  make([]LogEntry, 0, len(entries)),
		}
		for _, e := range entries {
			result.Entries = append(result.Entries, LogEntry{
				URI:            e.ODataID,
				ID:             e.ID,
				EntryType:      string(e.EntryType),
				Severity:       string(e.Severity),
				Message:        e.Message,
				MessageID:      e.MessageID,
				EventTimestamp: e.EventTimestamp,
				Created:        e.Created,
			})
		}
		return nil
	})
	if err != nil {
		return LogResult{}, err
	}
	return result, nil
}

func (r *LogReader) query(ctx context.Context, read func(*logClient) error) error {
	for attempt := range 2 {
		sdk, mode, err := r.connection.connectAuthenticated(ctx, nil)
		if err != nil {
			return err
		}
		c := &logClient{
			APIClient:  sdk.WithContext(ctx),
			connection: r.connection,
			ctx:        ctx,
		}
		c.Service.SetClient(c)
		err = read(c)
		// Cleanup is bounded even after the caller cancels the query.
		retireSession(context.WithoutCancel(ctx), sdk)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt == 0 && mode == "session" && c.unauthorized.Load() {
			continue
		}
		return err
	}
	return nil
}
