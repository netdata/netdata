// SPDX-License-Identifier: GPL-3.0-or-later

// Package syntheticfunc exposes read-only native synthetic observations.
package syntheticfunc

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Deps exposes snapshots and immutable retained evidence, without browser
// admission or a second desired-configuration registry.
type Deps interface {
	Snapshot(time.Time) []model.Job
	ArtifactStats() artifacts.Stats
	QuerySyntheticRuns(context.Context, model.RunFilter) (model.RunPage, error)
	GetSyntheticRun(context.Context, string, string) (model.Run, error)
	Manifest(context.Context, string) ([]model.Artifact, error)
	Fetch(context.Context, string, string) (model.Artifact, []byte, error)
}

type Handler struct{ deps Deps }

func New(deps Deps) *Handler {
	return &Handler{
		deps: deps,
	}
}
func (h *Handler) Cleanup(context.Context) {}
func (h *Handler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}
func (h *Handler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return funcapi.ErrorResponse(400, "raw synthetic request required")
}

func (h *Handler) HandleRaw(ctx context.Context, req funcapi.RawMethodRequest) *funcapi.FunctionResponse {
	var spec *method
	for i := range methods {
		if methods[i].id == req.Method {
			spec = &methods[i]
			break
		}
	}
	if spec == nil {
		return funcapi.NotFoundResponse(req.Method)
	}
	// Metadata discovery never queries active jobs, history or artifact storage.
	if req.Info {
		return &funcapi.FunctionResponse{
			Status:            200,
			Help:              spec.help,
			Columns:           spec.columns,
			DefaultSortColumn: spec.sort,
		}
	}
	if !permitted(req.Permissions, 0x1b) {
		return funcapi.ErrorResponse(403, "member permissions are required")
	}
	if len(req.Payload) > 0 {
		return funcapi.ErrorResponse(400, "this read-only Function does not accept a payload")
	}
	args, err := parseArgs(req.Args, spec.params)
	if err != nil {
		return funcapi.ErrorResponse(400, "%v", err)
	}
	if req.Method == "synthetics-artifact" && args["artifact_id"] != "" && !permitted(req.Permissions, 0x7b) {
		return funcapi.ErrorResponse(403, "administrator permissions are required for artifact bytes")
	}
	if err := ctx.Err(); err != nil {
		return functionError(err)
	}
	if h.deps == nil {
		return funcapi.UnavailableResponse("synthetic observations are unavailable")
	}
	response := map[string]any{
		"status":              200,
		"type":                "table",
		"has_history":         spec.history,
		"update_every":        10,
		"help":                spec.help,
		"columns":             spec.columns,
		"data":                [][]any{},
		"default_sort_column": spec.sort,
		"accepted_params":     spec.params,
	}
	switch req.Method {
	case "synthetics-checks":
		response["artifacts"] = h.deps.ArtifactStats()
		if err = validKind(args["kind"]); err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		limit, err := rowLimit(args["limit"])
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		rows := make([][]any, 0)
		truncated := false
		for _, j := range h.deps.Snapshot(time.Now()) {
			if args["job_id"] != "" && args["job_id"] != j.JobID {
				continue
			}
			if args["kind"] != "" && args["kind"] != string(j.Kind) {
				continue
			}
			if len(rows) == limit {
				truncated = true
				break
			}
			var id, outcome, detail, historyErr string
			var completed, duration any
			if last := j.Latest; last != nil {
				id = last.ID
				outcome = string(last.Outcome)
				detail = last.Error
				historyErr = last.HistoryError
				completed = timestamp(last.CompletedUS)
				duration = number(last.DurationMS)
			}
			rows = append(
				rows,
				[]any{
					j.JobID,
					j.Kind,
					j.Name,
					j.Target,
					j.State,
					j.Fresh,
					id,
					outcome,
					completed,
					duration,
					timestamp(j.LastSuccessUS),
					timestamp(j.LastFailureUS),
					detail,
					historyErr,
				},
			)
		}
		response["data"] = rows
		response["truncated"] = truncated
		response["limit"] = limit
	case "synthetics-runs":
		filter, err := runFilter(args, time.Now().Unix())
		if err != nil {
			return funcapi.ErrorResponse(400, "%v", err)
		}
		page, err := h.deps.QuerySyntheticRuns(ctx, filter)
		if err != nil {
			return functionError(err)
		}
		rows := make([][]any, 0, len(page.Runs))
		for _, run := range page.Runs {
			rows = append(
				rows,
				[]any{
					run.ID,
					run.JobID,
					run.Kind,
					run.Name,
					run.Target,
					timestamp(run.StartedUS),
					timestamp(run.CompletedUS),
					run.Outcome,
					number(run.DurationMS),
					run.CaptureState,
					run.Error,
					run.HistoryError,
				},
			)
		}
		response["data"] = rows
		response["truncated"] = page.Truncated
		response["limit"] = filter.Limit
		response["after"] = filter.After
		response["before"] = *filter.Before
	case "synthetics-run", "synthetics-artifact":
		if args["job_id"] == "" || args["run_id"] == "" {
			return funcapi.ErrorResponse(400, "job_id and run_id are required")
		}
		run, err := h.getRun(ctx, args["job_id"], args["run_id"])
		if err != nil {
			return functionError(err)
		}
		if req.Method == "synthetics-artifact" {
			return h.artifact(ctx, args, run, response)
		}
		rows := make([][]any, 0, len(run.Events))
		for _, e := range run.Events {
			rows = append(
				rows,
				[]any{
					e.AtMS,
					e.Kind,
					e.TestID,
					e.Title,
					e.Phase,
					e.Status,
					e.ExpectedStatus,
					number(e.DurationMS),
					e.Message,
				},
			)
		}
		response["data"] = rows
		response["run"] = runDetail(run)
		response["dropped_events"] = run.DroppedEvents
	}
	return funcapi.RawResponse(response)
}

func (h *Handler) getRun(ctx context.Context, jobID, runID string) (model.Run, error) {
	// The active snapshot preserves current diagnosis when journal publication
	// failed. A historical query must never manufacture a result from a job name.
	for _, job := range h.deps.Snapshot(time.Now()) {
		if job.JobID == jobID && job.Latest != nil && job.Latest.ID == runID {
			return *job.Latest, nil
		}
	}
	return h.deps.GetSyntheticRun(ctx, jobID, runID)
}

func (h *Handler) artifact(
	ctx context.Context,
	args map[string]string,
	run model.Run,
	response map[string]any,
) *funcapi.FunctionResponse {
	response["capture_state"] = run.CaptureState
	response["run_id"] = run.ID
	response["job_id"] = run.JobID
	if args["artifact_id"] != "" {
		known := false
		for _, a := range run.Artifacts {
			if a.ID == args["artifact_id"] {
				known = true
				break
			}
		}
		if !known {
			return funcapi.ErrorResponse(404, "artifact is not recorded for this run")
		}
		meta, body, err := h.deps.Fetch(ctx, run.ID, args["artifact_id"])
		if errors.Is(err, artifacts.ErrNotFound) {
			return funcapi.RawResponse(
				map[string]any{
					"status":        404,
					"errorMessage":  "recorded artifact is expired or unavailable",
					"capture_state": "expired_or_unavailable",
					"job_id":        run.JobID,
					"run_id":        run.ID,
				},
			)
		}
		if err != nil {
			return functionError(err)
		}
		response["data"] = [][]any{{meta.ID, meta.Kind, meta.MIME, meta.Bytes, meta.SHA256, "available"}}
		response["capture_state"] = "available"
		response["encoding"] = "base64"
		response["data_base64"] = base64.StdEncoding.EncodeToString(body)
		return funcapi.RawResponse(response)
	}
	if len(run.Artifacts) == 0 {
		return funcapi.RawResponse(response)
	}
	manifest, err := h.deps.Manifest(ctx, run.ID)
	if err != nil && !errors.Is(err, artifacts.ErrNotFound) {
		return functionError(err)
	}
	available := make(map[string]bool, len(manifest))
	for _, a := range manifest {
		available[a.ID] = true
	}
	rows := make([][]any, 0, len(run.Artifacts))
	for _, a := range run.Artifacts {
		state := "expired_or_unavailable"
		if available[a.ID] {
			state = "available"
		}
		rows = append(rows, []any{a.ID, a.Kind, a.MIME, a.Bytes, a.SHA256, state})
	}
	response["data"] = rows
	if errors.Is(err, artifacts.ErrNotFound) {
		response["capture_state"] = "expired_or_unavailable"
	}
	return funcapi.RawResponse(response)
}

func runDetail(run model.Run) map[string]any {
	detail := map[string]any{
		"id":             run.ID,
		"job_id":         run.JobID,
		"kind":           run.Kind,
		"name":           run.Name,
		"target":         run.Target,
		"started_us":     timestamp(run.StartedUS),
		"completed_us":   timestamp(run.CompletedUS),
		"duration_ms":    number(run.DurationMS),
		"outcome":        run.Outcome,
		"error":          run.Error,
		"tests":          run.Tests,
		"capture_state":  run.CaptureState,
		"artifacts":      run.Artifacts,
		"history_error":  run.HistoryError,
		"dropped_events": run.DroppedEvents,
		"metrics":        nil,
	}
	if m := run.Metrics; m != nil {
		detail["metrics"] = map[string]any{
			"performance": number(m.Performance),
			"fcp_ms":      number(m.FCPMS),
			"lcp_ms":      number(m.LCPMS),
			"tbt_ms":      number(m.TBTMS),
			"si_ms":       number(m.SIMS),
			"cls":         number(m.CLS),
		}
	}
	return detail
}
func permitted(value string, required uint64) bool {
	mask, err := strconv.ParseUint(value, 0, 64)
	return err == nil && mask&required == required
}
func parseArgs(words, accepted []string) (map[string]string, error) {
	args := make(map[string]string, len(words))
	for _, word := range words {
		key, value, ok := strings.Cut(word, ":")
		if !ok {
			return nil, errors.New("expected key:value argument")
		}
		allowed := false
		for _, name := range accepted {
			if key == name {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("unsupported argument %q", key)
		}
		if _, exists := args[key]; exists {
			return nil, fmt.Errorf("duplicate argument %q", key)
		}
		args[key] = value
	}
	return args, nil
}
func rowLimit(value string) (int, error) {
	if value == "" {
		return 2000, nil
	}
	limit, err := strconv.Atoi(value)
	if err != nil || limit < 1 || limit > 2000 {
		return 0, errors.New("limit must be between 1 and 2000")
	}
	return limit, nil
}
func validKind(value string) error {
	if value != "" && value != string(model.Journey) && value != string(model.Lighthouse) {
		return errors.New("kind must be journey or lighthouse")
	}
	return nil
}
func runFilter(args map[string]string, now int64) (model.RunFilter, error) {
	before := now
	f := model.RunFilter{
		JobID:   args["job_id"],
		Kind:    model.Kind(args["kind"]),
		Outcome: model.Outcome(args["outcome"]),
		Before:  &before,
	}
	if err := validKind(args["kind"]); err != nil {
		return f, err
	}
	switch f.Outcome {
	case "",
		model.Unknown,
		model.Success,
		model.Failed,
		model.Timeout,
		model.Inconclusive,
		model.Error,
		model.Cancelled:
	default:
		return f, errors.New("unknown outcome filter")
	}
	var err error
	f.Limit, err = rowLimit(args["limit"])
	if err != nil {
		return f, err
	}
	for key, dst := range map[string]*int64{"after": &f.After, "before": f.Before} {
		if args[key] != "" {
			v, err := strconv.ParseInt(args[key], 10, 64)
			if err != nil {
				return f, fmt.Errorf("%s must be Unix seconds or a relative negative offset", key)
			}
			if v < 0 {
				v += now
			}
			if v < 0 {
				return f, fmt.Errorf("%s must not precede the Unix epoch", key)
			}
			*dst = v
		}
	}
	if f.After > *f.Before {
		return f, errors.New("after must not exceed before")
	}
	return f, nil
}
func timestamp(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
func number(value *float64) any {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	return *value
}
func functionError(err error) *funcapi.FunctionResponse {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return funcapi.ErrorResponse(499, "request cancelled")
	case errors.Is(err, os.ErrNotExist), errors.Is(err, artifacts.ErrNotFound):
		return funcapi.ErrorResponse(404, "requested evidence is unavailable")
	case errors.Is(err, artifacts.ErrTooLarge):
		return funcapi.ErrorResponse(413, "artifact exceeds the fetch limit")
	case errors.Is(err, artifacts.ErrInvalidID):
		return funcapi.ErrorResponse(400, "invalid run or artifact identifier")
	default:
		return funcapi.InternalErrorResponse("synthetic evidence could not be read")
	}
}
