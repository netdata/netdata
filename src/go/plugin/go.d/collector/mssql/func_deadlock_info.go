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
	deadlockInfoHelp         = "Latest deadlock from the system_health Extended Events session. WARNING: query text may include unmasked sensitive literals; restrict dashboard access."
	deadlockParseErrorStatus = 561
)

const deadlockInfoMethodID = "deadlock-info"

// deadlockRowData holds computed values for a single deadlock row.
type deadlockRowData struct {
	rowID        string
	deadlockID   string
	timestamp    string
	processID    string
	spid         any
	ecid         any
	isVictim     string
	queryText    string
	lockMode     string
	lockStatus   string
	waitResource string
	database     any
}

// deadlockColumn defines a column for the deadlock-info function.
type deadlockColumn struct {
	funcapi.ColumnMeta
	Value func(*deadlockRowData) any
}

func deadlockColumnSet(cols []deadlockColumn) funcapi.ColumnSet[deadlockColumn] {
	return funcapi.Columns(cols, func(c deadlockColumn) funcapi.ColumnMeta { return c.ColumnMeta })
}

var deadlockColumns = []deadlockColumn{
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "row_id",
			Tooltip:   "Unique identifier for this row",
			Type:      funcapi.FieldTypeString,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  true,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterMultiselect,
			UniqueKey: true,
			Visible:   false,
		},
		Value: func(r *deadlockRowData) any { return r.rowID },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "timestamp",
			Tooltip:   "When the deadlock occurred",
			Type:      funcapi.FieldTypeTimestamp,
			Sort:      funcapi.FieldSortDescending,
			Sortable:  true,
			Summary:   funcapi.FieldSummaryMax,
			Filter:    funcapi.FieldFilterRange,
			Visible:   true,
			Transform: funcapi.FieldTransformDatetime,
		},
		Value: func(r *deadlockRowData) any { return r.timestamp },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "is_victim",
			Tooltip:       "Whether this process was rolled back to resolve the deadlock",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualPill,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
		},
		Value: func(r *deadlockRowData) any { return r.isVictim },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "query_text",
			Tooltip:   "The SQL statement being executed",
			Type:      funcapi.FieldTypeString,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  false,
			Sticky:    true,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterMultiselect,
			FullWidth: true,
			Wrap:      true,
			Visible:   true,
		},
		Value: func(r *deadlockRowData) any { return r.queryText },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "database",
			Tooltip:  "Database where the deadlock occurred",
			Type:     funcapi.FieldTypeString,
			Sort:     funcapi.FieldSortAscending,
			Sortable: true,
			Summary:  funcapi.FieldSummaryCount,
			Filter:   funcapi.FieldFilterMultiselect,
			Visible:  true,
		},
		Value: func(r *deadlockRowData) any { return r.database },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "lock_mode",
			Tooltip:  "Type of lock (S=Shared, X=Exclusive, U=Update, etc.)",
			Type:     funcapi.FieldTypeString,
			Sort:     funcapi.FieldSortAscending,
			Sortable: true,
			Summary:  funcapi.FieldSummaryCount,
			Filter:   funcapi.FieldFilterMultiselect,
			Visible:  true,
		},
		Value: func(r *deadlockRowData) any { return r.lockMode },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:          "lock_status",
			Tooltip:       "Whether the lock was granted or still waiting",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualPill,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
		},
		Value: func(r *deadlockRowData) any { return r.lockStatus },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "wait_resource",
			Tooltip:   "The resource this process was waiting to acquire",
			Type:      funcapi.FieldTypeString,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  false,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterMultiselect,
			FullWidth: true,
			Wrap:      true,
			Visible:   true,
		},
		Value: func(r *deadlockRowData) any { return r.waitResource },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "spid",
			Tooltip:   "Server Process ID (SQL Server session ID)",
			Type:      funcapi.FieldTypeInteger,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  true,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterRange,
			Visible:   true,
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *deadlockRowData) any { return r.spid },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "ecid",
			Tooltip:   "Execution Context ID for parallel query threads",
			Type:      funcapi.FieldTypeInteger,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  true,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterRange,
			Visible:   true,
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *deadlockRowData) any { return r.ecid },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "process_id",
			Tooltip:  "Internal process identifier from the deadlock graph",
			Type:     funcapi.FieldTypeString,
			Sort:     funcapi.FieldSortAscending,
			Sortable: true,
			Summary:  funcapi.FieldSummaryCount,
			Filter:   funcapi.FieldFilterMultiselect,
			Visible:  true,
		},
		Value: func(r *deadlockRowData) any { return r.processID },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:     "deadlock_id",
			Tooltip:  "Unique identifier for this deadlock event",
			Type:     funcapi.FieldTypeString,
			Sort:     funcapi.FieldSortAscending,
			Sortable: true,
			Summary:  funcapi.FieldSummaryCount,
			Filter:   funcapi.FieldFilterMultiselect,
			Visible:  true,
		},
		Value: func(r *deadlockRowData) any { return r.deadlockID },
	},
}

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
	return f.collectData(ctx)
}

func (f *funcDeadlockInfo) Cleanup(ctx context.Context) {}

func (f *funcDeadlockInfo) collectData(ctx context.Context) *funcapi.FunctionResponse {
	if !f.router.collector.Config.GetDeadlockInfoFunctionEnabled() {
		return &funcapi.FunctionResponse{
			Status: 503,
			Message: "deadlock-info function has been disabled in configuration. " +
				"To enable, set deadlock_info_function_enabled: true in the MSSQL collector config.",
		}
	}

	deadlockTime, deadlockXML, err := f.queryLatestDeadlock(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return f.buildResponse(504, "deadlock query timed out", nil)
		}
		if isDeadlockPermissionError(err) {
			return f.buildResponse(403, deadlockPermissionMessage(), nil)
		}
		f.router.collector.Warningf("deadlock-info: query failed: %v", err)
		return f.buildResponse(500, fmt.Sprintf("deadlock query failed: %v", err), nil)
	}

	if deadlockXML == "" {
		return f.buildResponse(200, "no deadlock found in system_health Extended Events", nil)
	}

	dbNames, dbErr := f.queryDatabaseNames(ctx)
	if dbErr != nil {
		f.router.collector.Debugf("deadlock-info: database name mapping failed: %v", dbErr)
		dbNames = map[int]string{}
	}

	parseRes := parseDeadlockGraph(deadlockXML, deadlockTime)
	if parseRes.parseErr != nil {
		f.router.collector.Warningf("deadlock-info: parse failed: %v", parseRes.parseErr)
		return f.buildResponse(deadlockParseErrorStatus, "deadlock graph could not be parsed", nil)
	}

	if !parseRes.found {
		return f.buildResponse(200, "no deadlock found in system_health Extended Events", nil)
	}

	deadlockID := generateDeadlockID(parseRes.deadlockTime)
	rows := buildDeadlockRows(parseRes, deadlockID, dbNames)

	if len(rows) == 0 {
		return f.buildResponse(200, "deadlock detected but no processes could be parsed", nil)
	}

	return f.buildResponse(200, "latest detected deadlock", rows)
}

func (f *funcDeadlockInfo) buildResponse(status int, message string, rowsData []deadlockRowData) *funcapi.FunctionResponse {
	data := make([][]any, 0, len(rowsData))
	for i := range rowsData {
		row := make([]any, len(deadlockColumns))
		for j, col := range deadlockColumns {
			row[j] = col.Value(&rowsData[i])
		}
		data = append(data, row)
	}

	cs := deadlockColumnSet(deadlockColumns)

	return &funcapi.FunctionResponse{
		Status:            status,
		Help:              deadlockInfoHelp,
		Message:           message,
		Columns:           cs.BuildColumns(),
		Data:              data,
		DefaultSortColumn: "timestamp",
	}
}

func (f *funcDeadlockInfo) queryLatestDeadlock(ctx context.Context) (time.Time, string, error) {
	qctx, cancel := context.WithTimeout(ctx, f.router.collector.Timeout.Duration())
	defer cancel()

	query := querySystemHealthLatestDeadlockEventFile
	if f.router.collector.Config.GetDeadlockInfoUseRingBuffer() {
		query = querySystemHealthLatestDeadlockRingBuffer
	}

	var deadlockTime sql.NullTime
	var deadlockXML sql.NullString
	err := f.router.collector.db.QueryRowContext(qctx, query).Scan(&deadlockTime, &deadlockXML)
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

func (f *funcDeadlockInfo) queryDatabaseNames(ctx context.Context) (map[int]string, error) {
	qctx, cancel := context.WithTimeout(ctx, f.router.collector.Timeout.Duration())
	defer cancel()

	rows, err := f.router.collector.db.QueryContext(qctx, queryDatabaseNamesByID)
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

func buildDeadlockRows(parseRes mssqlDeadlockParseResult, deadlockID string, dbNames map[int]string) []deadlockRowData {
	timestamp := parseRes.deadlockTime.UTC().Format(time.RFC3339Nano)
	rows := make([]deadlockRowData, 0, len(parseRes.transactions))

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
		lockMode := formatLockMode(strings.TrimSpace(txn.lockMode))
		lockStatus := strings.TrimSpace(txn.lockStatus)
		waitResource := strmutil.TruncateText(strings.TrimSpace(txn.waitResource), topQueriesMaxTextLength)

		rows = append(rows, deadlockRowData{
			rowID:        fmt.Sprintf("%s:%s", deadlockID, processID),
			deadlockID:   deadlockID,
			timestamp:    timestamp,
			processID:    processID,
			spid:         spid,
			ecid:         ecid,
			isVictim:     isVictim,
			queryText:    queryText,
			lockMode:     lockMode,
			lockStatus:   lockStatus,
			waitResource: waitResource,
			database:     database,
		})
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

func formatLockMode(mode string) string {
	names := map[string]string{
		"X":     "Exclusive",
		"S":     "Shared",
		"U":     "Update",
		"IX":    "Intent Exclusive",
		"IS":    "Intent Shared",
		"SIX":   "Shared Intent Exclusive",
		"Sch-S": "Schema Stability",
		"Sch-M": "Schema Modification",
		"BU":    "Bulk Update",
	}
	if name, ok := names[mode]; ok {
		return fmt.Sprintf("%s (%s)", name, mode)
	}
	return mode
}
