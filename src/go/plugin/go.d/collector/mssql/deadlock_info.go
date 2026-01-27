// SPDX-License-Identifier: GPL-3.0-or-later

package mssql

import (
	"context"
	"database/sql"
	"encoding/xml"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	mssqlDriver "github.com/microsoft/go-mssqldb"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const (
	deadlockInfoHelp         = "Latest deadlock from the system_health Extended Events ring buffer. WARNING: query text may include unmasked sensitive literals; restrict dashboard access."
	deadlockParseErrorStatus = 561
)

const deadlockInfoMethodID = "deadlock-info"

func deadlockInfoMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:             deadlockInfoMethodID,
		Name:           "Deadlock Info",
		UpdateEvery:    10,
		Help:           deadlockInfoHelp,
		RequireCloud:   true,
		RequiredParams: []funcapi.ParamConfig{},
	}
}

// funcDeadlockInfo handles the deadlock-info function.
type funcDeadlockInfo struct {
	router *funcRouter
}

func newFuncDeadlockInfo(r *funcRouter) *funcDeadlockInfo {
	return &funcDeadlockInfo{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcDeadlockInfo)(nil)

func (f *funcDeadlockInfo) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if !f.router.collector.Config.GetDeadlockInfoFunctionEnabled() {
		return nil, fmt.Errorf("deadlock-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (f *funcDeadlockInfo) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if f.router.collector.db == nil {
		db, err := f.router.collector.openConnection()
		if err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
		f.router.collector.db = db
	}
	return f.router.collector.collectDeadlockInfo(ctx)
}

func (f *funcDeadlockInfo) Cleanup(ctx context.Context) {}

const (
	deadlockIdxRowID = iota
	deadlockIdxDeadlockID
	deadlockIdxTimestamp
	deadlockIdxProcessID
	deadlockIdxSpid
	deadlockIdxEcid
	deadlockIdxIsVictim
	deadlockIdxQueryText
	deadlockIdxLockMode
	deadlockIdxLockStatus
	deadlockIdxWaitResource
	deadlockIdxDatabase
	deadlockColumnCount
)

type mssqlDeadlockTxn struct {
	processID    string
	spid         string
	ecid         string
	dbid         string
	queryText    string
	lockMode     string
	lockStatus   string
	waitResource string
}

type mssqlDeadlockParseResult struct {
	deadlockTime    time.Time
	transactions    []*mssqlDeadlockTxn
	victimProcessID string
	parseErr        error
	found           bool
}

type mssqlDeadlockGraph struct {
	XMLName      xml.Name                  `xml:"deadlock"`
	VictimList   mssqlDeadlockVictimList   `xml:"victim-list"`
	ProcessList  mssqlDeadlockProcessList  `xml:"process-list"`
	ResourceList mssqlDeadlockResourceList `xml:"resource-list"`
}

type mssqlDeadlockResourceList struct {
	Resources []mssqlDeadlockResource `xml:",any"`
}

type mssqlDeadlockVictimList struct {
	Victims []mssqlDeadlockVictim `xml:"victimProcess"`
}

type mssqlDeadlockVictim struct {
	ID string `xml:"id,attr"`
}

type mssqlDeadlockProcessList struct {
	Processes []mssqlDeadlockProcess `xml:"process"`
}

type mssqlDeadlockProcess struct {
	ID           string `xml:"id,attr"`
	SPID         string `xml:"spid,attr"`
	ECID         string `xml:"ecid,attr"`
	DBID         string `xml:"dbid,attr"`
	LockMode     string `xml:"lockMode,attr"`
	WaitResource string `xml:"waitresource,attr"`
	InputBuf     string `xml:"inputbuf"`
}

type mssqlDeadlockResource struct {
	XMLName    xml.Name
	DBID       string                  `xml:"dbid,attr"`
	OwnerList  mssqlDeadlockOwnerList  `xml:"owner-list"`
	WaiterList mssqlDeadlockWaiterList `xml:"waiter-list"`
}

type mssqlDeadlockOwnerList struct {
	Owners []mssqlDeadlockResourceEntry `xml:"owner"`
}

type mssqlDeadlockWaiterList struct {
	Waiters []mssqlDeadlockResourceEntry `xml:"waiter"`
}

type mssqlDeadlockResourceEntry struct {
	ID   string `xml:"id,attr"`
	Mode string `xml:"mode,attr"`
}

func (c *Collector) deadlockInfoParams(context.Context) ([]funcapi.ParamConfig, error) {
	if !c.Config.GetDeadlockInfoFunctionEnabled() {
		return nil, fmt.Errorf("deadlock-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (c *Collector) collectDeadlockInfo(ctx context.Context) *funcapi.FunctionResponse {
	if !c.Config.GetDeadlockInfoFunctionEnabled() {
		return &funcapi.FunctionResponse{
			Status: 503,
			Message: "deadlock-info function has been disabled in configuration. " +
				"To enable, set deadlock_info_function_enabled: true in the MSSQL collector config.",
		}
	}

	deadlockTime, deadlockXML, err := c.queryLatestDeadlock(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return c.deadlockInfoResponse(504, "deadlock query timed out", nil)
		}
		if isDeadlockPermissionError(err) {
			return c.deadlockInfoResponse(403, deadlockPermissionMessage(), nil)
		}
		c.Warningf("deadlock-info: query failed: %v", err)
		return c.deadlockInfoResponse(500, fmt.Sprintf("deadlock query failed: %v", err), nil)
	}

	if deadlockXML == "" {
		return c.deadlockInfoResponse(200, "no deadlock found in system_health ring buffer", nil)
	}

	dbNames, dbErr := c.queryDatabaseNames(ctx)
	if dbErr != nil {
		c.Debugf("deadlock-info: database name mapping failed: %v", dbErr)
		dbNames = map[int]string{}
	}

	parseRes := parseDeadlockGraph(deadlockXML, deadlockTime)
	if parseRes.parseErr != nil {
		c.Warningf("deadlock-info: parse failed: %v", parseRes.parseErr)
		return c.deadlockInfoResponse(deadlockParseErrorStatus, "deadlock graph could not be parsed", nil)
	}
	if !parseRes.found {
		return c.deadlockInfoResponse(200, "no deadlock found in system_health ring buffer", nil)
	}

	deadlockID := generateDeadlockID(parseRes.deadlockTime)
	rows := buildDeadlockRows(parseRes, deadlockID, dbNames)
	if len(rows) == 0 {
		return c.deadlockInfoResponse(200, "deadlock detected but no processes could be parsed", nil)
	}

	return c.deadlockInfoResponse(200, "latest detected deadlock", rows)
}

func (c *Collector) deadlockInfoResponse(status int, message string, data [][]any) *funcapi.FunctionResponse {
	if data == nil {
		data = make([][]any, 0)
	}
	return &funcapi.FunctionResponse{
		Status:            status,
		Help:              deadlockInfoHelp,
		Message:           message,
		Columns:           c.buildDeadlockColumns(),
		Data:              data,
		DefaultSortColumn: "timestamp",
	}
}

func (c *Collector) queryLatestDeadlock(ctx context.Context) (time.Time, string, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	var deadlockTime sql.NullTime
	var deadlockXML sql.NullString
	err := c.db.QueryRowContext(qctx, querySystemHealthLatestDeadlock).Scan(&deadlockTime, &deadlockXML)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, "", nil
		}
		return time.Time{}, "", err
	}

	if !deadlockXML.Valid || strings.TrimSpace(deadlockXML.String) == "" {
		return time.Time{}, "", nil
	}

	if deadlockTime.Valid {
		return deadlockTime.Time, deadlockXML.String, nil
	}
	return time.Now().UTC(), deadlockXML.String, nil
}

func (c *Collector) queryDatabaseNames(ctx context.Context) (map[int]string, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	rows, err := c.db.QueryContext(qctx, queryDatabaseNamesByID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	names := make(map[int]string)
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		names[id] = name
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func (c *Collector) buildDeadlockColumns() map[string]any {
	const (
		ftString    = funcapi.FieldTypeString
		ftInteger   = funcapi.FieldTypeInteger
		ftTimestamp = funcapi.FieldTypeTimestamp

		trNone     = funcapi.FieldTransformNone
		trNumber   = funcapi.FieldTransformNumber
		trDatetime = funcapi.FieldTransformDatetime

		visValue = funcapi.FieldVisualValue
		visPill  = funcapi.FieldVisualPill

		sortAsc  = funcapi.FieldSortAscending
		sortDesc = funcapi.FieldSortDescending

		summaryCount = funcapi.FieldSummaryCount
		summaryMax   = funcapi.FieldSummaryMax

		filterMulti = funcapi.FieldFilterMultiselect
		filterRange = funcapi.FieldFilterRange
	)

	return map[string]any{
		"row_id": funcapi.Column{
			Index:         deadlockIdxRowID,
			Name:          "Row ID",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Summary:       summaryCount,
			Filter:        filterMulti,
			UniqueKey:     true,
			Visible:       false,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"deadlock_id": funcapi.Column{
			Index:         deadlockIdxDeadlockID,
			Name:          "Deadlock ID",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortDesc,
			Sortable:      true,
			Sticky:        true,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"timestamp": funcapi.Column{
			Index:         deadlockIdxTimestamp,
			Name:          "Timestamp",
			Type:          ftTimestamp,
			Units:         "",
			Visualization: visValue,
			Sort:          sortDesc,
			Sortable:      true,
			Sticky:        true,
			Summary:       summaryMax,
			Filter:        filterRange,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trDatetime,
			},
		}.BuildColumn(),
		"process_id": funcapi.Column{
			Index:         deadlockIdxProcessID,
			Name:          "Process ID",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        true,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"spid": funcapi.Column{
			Index:         deadlockIdxSpid,
			Name:          "SPID",
			Type:          ftInteger,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterRange,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNumber,
			},
		}.BuildColumn(),
		"ecid": funcapi.Column{
			Index:         deadlockIdxEcid,
			Name:          "ECID",
			Type:          ftInteger,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterRange,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNumber,
			},
		}.BuildColumn(),
		"is_victim": funcapi.Column{
			Index:         deadlockIdxIsVictim,
			Name:          "Victim",
			Type:          ftString,
			Visualization: visPill,
			Sort:          sortAsc,
			Sortable:      true,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"query_text": funcapi.Column{
			Index:         deadlockIdxQueryText,
			Name:          "Query",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      false,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			FullWidth:     true,
			Wrap:          true,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"lock_mode": funcapi.Column{
			Index:         deadlockIdxLockMode,
			Name:          "Lock Mode",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"lock_status": funcapi.Column{
			Index:         deadlockIdxLockStatus,
			Name:          "Lock Status",
			Type:          ftString,
			Visualization: visPill,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"wait_resource": funcapi.Column{
			Index:         deadlockIdxWaitResource,
			Name:          "Wait Resource",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      false,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			FullWidth:     true,
			Wrap:          true,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"database": funcapi.Column{
			Index:         deadlockIdxDatabase,
			Name:          "Database",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
	}
}

func parseDeadlockGraph(deadlockXML string, deadlockTime time.Time) mssqlDeadlockParseResult {
	now := time.Now().UTC()
	result := mssqlDeadlockParseResult{
		deadlockTime: now,
		found:        strings.TrimSpace(deadlockXML) != "",
	}
	if result.found && !deadlockTime.IsZero() {
		result.deadlockTime = deadlockTime.UTC()
	}

	if !result.found {
		return result
	}

	var graph mssqlDeadlockGraph
	if err := xml.Unmarshal([]byte(deadlockXML), &graph); err != nil {
		result.parseErr = fmt.Errorf("failed to parse deadlock XML: %w", err)
		return result
	}

	if len(graph.VictimList.Victims) > 0 {
		result.victimProcessID = strings.TrimSpace(graph.VictimList.Victims[0].ID)
	}

	txnByID := make(map[string]*mssqlDeadlockTxn)
	ensureTxn := func(id string) *mssqlDeadlockTxn {
		if id == "" {
			return &mssqlDeadlockTxn{}
		}
		if txn, ok := txnByID[id]; ok {
			return txn
		}
		txn := &mssqlDeadlockTxn{processID: id}
		txnByID[id] = txn
		return txn
	}

	for _, proc := range graph.ProcessList.Processes {
		processID := strings.TrimSpace(proc.ID)
		if processID == "" {
			continue
		}
		txn := ensureTxn(processID)
		txn.spid = strings.TrimSpace(proc.SPID)
		txn.ecid = strings.TrimSpace(proc.ECID)
		txn.dbid = strings.TrimSpace(proc.DBID)
		txn.queryText = strings.TrimSpace(proc.InputBuf)
		txn.lockMode = strings.TrimSpace(proc.LockMode)
		txn.waitResource = strings.TrimSpace(proc.WaitResource)
		if txn.waitResource != "" {
			txn.lockStatus = "WAITING"
		} else if txn.lockStatus == "" {
			txn.lockStatus = "GRANTED"
		}
	}

	for _, resource := range graph.ResourceList.Resources {
		resourceDBID := strings.TrimSpace(resource.DBID)
		for _, owner := range resource.OwnerList.Owners {
			id := strings.TrimSpace(owner.ID)
			if id == "" {
				continue
			}
			txn := ensureTxn(id)
			if txn.dbid == "" && resourceDBID != "" {
				txn.dbid = resourceDBID
			}
			if txn.lockStatus != "WAITING" {
				if txn.lockStatus == "" {
					txn.lockStatus = "GRANTED"
				}
				mode := strings.TrimSpace(owner.Mode)
				if mode != "" {
					txn.lockMode = mode
				}
			}
		}
		for _, waiter := range resource.WaiterList.Waiters {
			id := strings.TrimSpace(waiter.ID)
			if id == "" {
				continue
			}
			txn := ensureTxn(id)
			if txn.dbid == "" && resourceDBID != "" {
				txn.dbid = resourceDBID
			}
			txn.lockStatus = "WAITING"
			mode := strings.TrimSpace(waiter.Mode)
			if mode != "" {
				txn.lockMode = mode
			}
		}
	}

	if len(txnByID) == 0 {
		result.parseErr = fmt.Errorf("deadlock graph detected but no processes could be parsed")
		return result
	}

	result.transactions = make([]*mssqlDeadlockTxn, 0, len(txnByID))
	for _, txn := range txnByID {
		if txn.processID == "" {
			continue
		}
		if txn.lockStatus == "" {
			if strings.TrimSpace(txn.waitResource) != "" {
				txn.lockStatus = "WAITING"
			} else {
				txn.lockStatus = "GRANTED"
			}
		}
		result.transactions = append(result.transactions, txn)
	}

	sort.Slice(result.transactions, func(i, j int) bool {
		return result.transactions[i].processID < result.transactions[j].processID
	})

	if len(result.transactions) == 0 {
		result.parseErr = fmt.Errorf("deadlock graph detected but no valid processes could be parsed")
	}

	return result
}

func buildDeadlockRows(parseRes mssqlDeadlockParseResult, deadlockID string, dbNames map[int]string) [][]any {
	timestamp := parseRes.deadlockTime.UTC().Format(time.RFC3339Nano)
	rows := make([][]any, 0, len(parseRes.transactions))

	for _, txn := range parseRes.transactions {
		processID := strings.TrimSpace(txn.processID)
		if processID == "" {
			continue
		}

		spid := parseOptionalInt(txn.spid)
		ecid := parseOptionalInt(txn.ecid)
		dbidInt, hasDBID := parseIntString(txn.dbid)

		var database any
		if hasDBID {
			if name, ok := dbNames[dbidInt]; ok {
				database = name
			}
		}

		isVictim := "false"
		if parseRes.victimProcessID != "" && processID == parseRes.victimProcessID {
			isVictim = "true"
		}

		queryText := strmutil.TruncateText(strings.TrimSpace(txn.queryText), topQueriesMaxTextLength)
		lockMode := strings.TrimSpace(txn.lockMode)
		lockStatus := strings.TrimSpace(txn.lockStatus)
		waitResource := strmutil.TruncateText(strings.TrimSpace(txn.waitResource), topQueriesMaxTextLength)

		row := make([]any, deadlockColumnCount)
		row[deadlockIdxRowID] = fmt.Sprintf("%s:%s", deadlockID, processID)
		row[deadlockIdxDeadlockID] = deadlockID
		row[deadlockIdxTimestamp] = timestamp
		row[deadlockIdxProcessID] = processID
		row[deadlockIdxSpid] = spid
		row[deadlockIdxEcid] = ecid
		row[deadlockIdxIsVictim] = isVictim
		row[deadlockIdxQueryText] = queryText
		row[deadlockIdxLockMode] = lockMode
		row[deadlockIdxLockStatus] = lockStatus
		row[deadlockIdxWaitResource] = waitResource
		row[deadlockIdxDatabase] = database

		rows = append(rows, row)
	}

	return rows
}

func parseIntString(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseOptionalInt(s string) any {
	if n, ok := parseIntString(s); ok {
		return n
	}
	return nil
}

func generateDeadlockID(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()
	micros := t.Nanosecond() / 1000
	return t.Format("20060102150405") + fmt.Sprintf("%06d", micros)
}

func isDeadlockPermissionError(err error) bool {
	var sqlErr mssqlDriver.Error
	if errors.As(err, &sqlErr) {
		if sqlErr.Number == 297 || sqlErr.Number == 229 {
			return true
		}
		if permissionMessage := strings.ToLower(sqlErr.Message); permissionMessage != "" {
			if strings.Contains(permissionMessage, "view server state") ||
				strings.Contains(permissionMessage, "permission") ||
				strings.Contains(permissionMessage, "denied") {
				return true
			}
		}
	}

	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "view server state") ||
		strings.Contains(msg, "permission") ||
		strings.Contains(msg, "denied")
}

func deadlockPermissionMessage() string {
	return "deadlock info requires VIEW SERVER STATE permission. Grant with: GRANT VIEW SERVER STATE TO [netdata_user];"
}
