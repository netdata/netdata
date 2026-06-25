// SPDX-License-Identifier: GPL-3.0-or-later

package snmptrapsfunc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdkjournal "github.com/netdata/systemd-journal-sdk/go/journal"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	LogsMethodID           = "logs"
	FunctionName           = "snmp:traps"
	logsSourceSelectorName = "Trap Jobs"
	logsSourceSelectorHelp = "Select the trap jobs to query"
)

type Handler struct {
	logs        sdkjournal.NetdataJournalFunction
	journalRoot string
}

var _ funcapi.RawMethodHandler = (*Handler)(nil)

func NewHandler(journalRoot string) *Handler {
	return &Handler{
		logs:        NewJournalFunction(),
		journalRoot: journalRoot,
	}
}

func LogsFunctionConfig(available func() bool) funcapi.FunctionConfig {
	return funcapi.FunctionConfig{
		ID:           LogsMethodID,
		FunctionName: FunctionName,
		Name:         "SNMP Trap Logs",
		UpdateEvery:  1,
		Help:         "Query SNMP trap journal entries received by SNMP trap listener jobs",
		RequireCloud: true,
		Tags:         "logs",
		ResponseType: "logs",
		Available:    available,
		RawRequest:   true,
	}
}

func (h *Handler) MethodParams(_ context.Context, _ string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (h *Handler) Handle(_ context.Context, method string, _ funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.NotFoundResponse(method)
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if req.Method != LogsMethodID {
		return funcapi.NotFoundResponse(req.Method)
	}
	root := h.journalRoot
	if err := validateLogsJournalRoot(root); err != nil {
		return funcapi.UnavailableResponse(err.Error())
	}
	payload := netdataLogsRequestPayload(req)
	if err := validateNetdataLogsRequestPayload(payload); err != nil {
		return funcapi.ErrorResponse(400, "SNMP trap logs query failed: %v", err)
	}

	resp, err := h.logs.RunDirectoryRequestBytesWithOptions(
		root,
		payload,
		netdataLogsRunOptions(ctx, req.Timeout, root),
	)
	if err != nil {
		return funcapi.ErrorResponse(400, "SNMP trap logs query failed: %v", err)
	}
	tuneSNMPTrapsLogsResponse(resp)
	return funcapi.RawResponse(resp)
}

func (h *Handler) Cleanup(context.Context) {
	// No owned resources: the SDK journal function is stateless between calls.
}

func NewJournalFunction() sdkjournal.NetdataJournalFunction {
	cfg := sdkjournal.SystemdJournalNetdataFunctionConfig()
	cfg.FunctionName = FunctionName
	cfg.SourceSelectorName = logsSourceSelectorName
	cfg.SourceSelectorHelp = logsSourceSelectorHelp
	cfg.DefaultFacets = DefaultLogFacets()
	cfg.DefaultViewKeys = DefaultViewKeys()
	cfg.DefaultHistogram = "TRAP_NAME"
	return sdkjournal.NewNetdataJournalFunction(cfg, sdkjournal.SystemdJournalProfile{})
}

func DefaultLogFacets() []string {
	return []string{
		"TRAP_CATEGORY",
		"TRAP_DEVICE_VENDOR",
		"TRAP_NAME",
		"TRAP_SEVERITY",
		"TRAP_SOURCE_IP",
		"_HOSTNAME",
		"TRAP_JOB",
	}
}

func DefaultViewKeys() []string {
	return []string{
		"MESSAGE",
		"_HOSTNAME",
		"TRAP_NAME",
		"TRAP_SEVERITY",
		"TRAP_CATEGORY",
		"TRAP_JOB",
		"TRAP_DECODE_ERROR_KIND",
		"TRAP_DECODE_ERROR",
		"TRAP_SOURCE_IP",
		"TRAP_SOURCE_UDP_PEER",
		"TRAP_SOURCE_UDP_PORT",
		"TRAP_REVERSE_DNS",
		"TRAP_DEVICE_VENDOR",
		"TRAP_OID",
		"TRAP_PACKET_SIZE",
		"TRAP_PACKET_SHA256",
		"TRAP_JSON",
	}
}

func netdataLogsRequestPayload(req funcapi.RawMethodRequest) []byte {
	if req.Info {
		return []byte(`{"info":true}`)
	}
	if len(bytes.TrimSpace(req.Payload)) == 0 {
		return []byte(`{}`)
	}
	return normalizeSNMPTrapsLogsRequestPayload(req.Payload)
}

func normalizeSNMPTrapsLogsRequestPayload(payload []byte) []byte {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		return payload
	}
	facetsRaw, ok := object["facets"]
	if !ok {
		return payload
	}
	var facets []json.RawMessage
	if err := json.Unmarshal(facetsRaw, &facets); err != nil || len(facets) != 0 {
		return payload
	}
	defaultFacets, err := json.Marshal(DefaultLogFacets())
	if err != nil {
		return payload
	}
	object["facets"] = defaultFacets
	normalized, err := json.Marshal(object)
	if err != nil {
		return payload
	}
	return normalized
}

func tuneSNMPTrapsLogsResponse(resp map[string]any) {
	setSNMPTrapsColumnVisible(resp, "TRAP_NAME")
}

func setSNMPTrapsColumnVisible(resp map[string]any, key string) {
	columns, ok := resp["columns"].(map[string]any)
	if !ok {
		return
	}
	column, ok := columns[key].(map[string]any)
	if !ok {
		return
	}
	column["visible"] = true
}

func netdataLogsRunOptions(ctx context.Context, timeout time.Duration, root string) sdkjournal.NetdataFunctionRunOptions {
	opts := sdkjournal.DefaultNetdataFunctionRunOptions()
	if timeout > 0 {
		opts.Timeout = &timeout
	}
	opts.State = logsState{root: root}
	opts.CancellationCallback = func() bool {
		return ctx.Err() != nil
	}
	return opts
}

func validateLogsJournalRoot(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("SNMP trap direct journal output has no sources")
		}
		return errors.New("SNMP trap direct journal output is unavailable: " + err.Error())
	}
	if !info.IsDir() {
		return errors.New("SNMP trap direct journal output path is not a directory")
	}
	return nil
}

func validateNetdataLogsRequestPayload(payload []byte) error {
	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return err
	}
	if _, ok := value.(map[string]any); !ok {
		return errors.New("Netdata function request must be a JSON object")
	}
	return nil
}

type logsState struct {
	root string
}

func (s logsState) FileMetadata(path string) *sdkjournal.NetdataJournalFileMetadata {
	rel, err := filepath.Rel(s.root, path)
	if err != nil {
		return nil
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil
	}

	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) == 0 || parts[0] == "" {
		return nil
	}

	sourceType := uint64(sdkjournal.NetdataSourceTypeAll)
	return &sdkjournal.NetdataJournalFileMetadata{
		SourceType: &sourceType,
		SourceName: parts[0],
	}
}

func (s logsState) UpdateFileJournalVsRealtimeDeltaUsec(string, uint64) {
	// SNMP trap logs do not publish per-file reader lag metrics today.
}
