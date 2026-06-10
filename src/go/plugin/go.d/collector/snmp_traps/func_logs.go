// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

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

func newSNMPTrapsFunctionHandler(c *Collector) *snmpTrapsFunctionHandler {
	return &snmpTrapsFunctionHandler{
		collector:   c,
		reload:      &profileReloadHandler{collector: c},
		logs:        newSNMPTrapsJournalFunction(),
		journalRoot: journalBaseRoot(),
	}
}

type snmpTrapsFunctionHandler struct {
	collector   *Collector
	reload      *profileReloadHandler
	logs        sdkjournal.NetdataJournalFunction
	journalRoot string
}

var _ funcapi.RawMethodHandler = (*snmpTrapsFunctionHandler)(nil)

func (h *snmpTrapsFunctionHandler) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method == reloadProfilesMethodID {
		return h.reload.MethodParams(ctx, method)
	}
	return nil, nil
}

func (h *snmpTrapsFunctionHandler) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method == reloadProfilesMethodID {
		return h.reload.Handle(ctx, method, params)
	}
	return funcapi.NotFoundResponse(method)
}

func (h *snmpTrapsFunctionHandler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	if !h.isLogsMethod(req.Method) {
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
	return funcapi.RawResponse(resp)
}

func (h *snmpTrapsFunctionHandler) Cleanup(ctx context.Context) {
	h.reload.Cleanup(ctx)
}

func (h *snmpTrapsFunctionHandler) isLogsMethod(method string) bool {
	return method == snmpTrapsLogsMethodID
}

func newSNMPTrapsJournalFunction() sdkjournal.NetdataJournalFunction {
	cfg := sdkjournal.SystemdJournalNetdataFunctionConfig()
	cfg.FunctionName = snmpTrapsFunctionName
	cfg.DefaultFacets = []string{
		"TRAP_REPORT_TYPE",
		"TRAP_DECODE_ERROR_KIND",
		"TRAP_SEVERITY",
		"TRAP_CATEGORY",
		"TRAP_NAME",
		"TRAP_DEVICE_VENDOR",
		"_HOSTNAME",
		"TRAP_SOURCE_IP",
		"TRAP_SOURCE_UDP_PEER",
		"TRAP_SOURCE_UDP_PORT",
		"TRAP_LISTENER",
		"TRAP_ENGINE_ID",
		"TRAP_OID",
		"TRAP_PDU_TYPE",
		"TRAP_VERSION",
		"ND_NIDL_NODE",
	}
	cfg.DefaultViewKeys = []string{
		"MESSAGE",
		"TRAP_SEVERITY",
		"TRAP_CATEGORY",
		"TRAP_NAME",
		"TRAP_DECODE_ERROR_KIND",
		"TRAP_DECODE_ERROR",
		"_HOSTNAME",
		"TRAP_SOURCE_IP",
		"TRAP_SOURCE_UDP_PEER",
		"TRAP_SOURCE_UDP_PORT",
		"TRAP_DEVICE_VENDOR",
		"TRAP_OID",
		"TRAP_PACKET_SIZE",
		"TRAP_PACKET_SHA256",
		"TRAP_JSON",
	}
	cfg.DefaultHistogram = "TRAP_SEVERITY"
	return sdkjournal.NewNetdataJournalFunction(cfg, sdkjournal.SystemdJournalProfile{})
}

func netdataLogsRequestPayload(req funcapi.RawMethodRequest) []byte {
	if req.Info {
		return []byte(`{"info":true}`)
	}
	if len(bytes.TrimSpace(req.Payload)) == 0 {
		return []byte(`{}`)
	}
	return req.Payload
}

func netdataLogsRunOptions(ctx context.Context, timeout time.Duration, root string) sdkjournal.NetdataFunctionRunOptions {
	opts := sdkjournal.DefaultNetdataFunctionRunOptions()
	if timeout > 0 {
		opts.Timeout = &timeout
	}
	opts.State = snmpTrapsLogsState{root: root}
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

type snmpTrapsLogsState struct {
	root string
}

func (s snmpTrapsLogsState) FileMetadata(path string) *sdkjournal.NetdataJournalFileMetadata {
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

func (s snmpTrapsLogsState) UpdateFileJournalVsRealtimeDeltaUsec(string, uint64) {}
