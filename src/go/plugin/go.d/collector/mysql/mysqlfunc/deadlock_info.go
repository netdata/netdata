// SPDX-License-Identifier: GPL-3.0-or-later

package mysqlfunc

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/strmutil"
)

const deadlockInfoMethodID = "deadlock-info"
const queryShowEngineInnoDBStatus = "SHOW ENGINE INNODB STATUS;"

const (
	deadlockSectionWaiting = "waiting"
	deadlockSectionHolds   = "holds"
)

var (
	reDeadlockHeader     = regexp.MustCompile(`LATEST DETECTED DEADLOCK`)
	reDeadlockTxn        = regexp.MustCompile(`(?i)^\*\*\* \((\d+)\) TRANSACTION:?`)
	reDeadlockWait       = regexp.MustCompile(`(?i)^\*\*\* \((\d+)\) WAITING FOR THIS LOCK TO BE GRANTED:?`)
	reDeadlockHolds      = regexp.MustCompile(`(?i)^\*\*\* \((\d+)\) HOLDS THE LOCK\(S\):?`)
	reDeadlockWaitNoTxn  = regexp.MustCompile(`(?i)^\*\*\*\s*WAITING FOR THIS LOCK TO BE GRANTED:?`)
	reDeadlockHoldsNoTxn = regexp.MustCompile(`(?i)^\*\*\*\s*HOLDS THE LOCK\(S\):?`)
	reDeadlockVictim     = regexp.MustCompile(`(?i)^\*\*\* WE ROLL BACK TRANSACTION \((\d+)\)`)
	reDeadlockThread     = regexp.MustCompile(`(?i)\b(?:mysql|mariadb)?\s*thread id\s+(\d+)`)
	reDeadlockMode       = regexp.MustCompile(`(?i)lock[_ ]mode\s+([A-Z0-9_-]+)`)
	reDeadlockTS         = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\b`)
	reDeadlockTable      = regexp.MustCompile(`(?i)\bof\s+table\s+` + "`?" + `([-\w$]+)` + "`?" + `\.` + "`?" + `([-\w$]+)` + "`?")
	reQueryTableRef      = regexp.MustCompile(`(?i)\b(?:from|update|into|join)\s+` + "`?" + `([-\w$]+)` + "`?" + `\.` + "`?" + `([-\w$]+)` + "`?")
	reSQLStatement       = regexp.MustCompile(`(?i)^(?:/\*.*\*/\s*)?(SELECT|UPDATE|INSERT|DELETE|REPLACE|WITH|ALTER|CREATE|DROP|TRUNCATE|LOCK|UNLOCK|SET|SHOW|CALL|EXEC|EXECUTE|DO|BEGIN|COMMIT|ROLLBACK|MERGE)\b`)
)

const (
	deadlockInfoHelp         = "Latest detected deadlock from SHOW ENGINE INNODB STATUS. WARNING: query text may include unmasked sensitive literals; restrict dashboard access."
	deadlockParseErrorStatus = 561
)

// deadlockRowData holds computed values for a single deadlock row.
type deadlockRowData struct {
	rowID        string
	deadlockID   string
	timestamp    string
	processID    string
	spid         any
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
			Tooltip:       "Whether this transaction was rolled back to resolve the deadlock",
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
			Tooltip:  "Type of lock (S=Shared, X=Exclusive, IS/IX=Intent locks)",
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
			Name:     "wait_resource",
			Tooltip:  "The resource this transaction was waiting to acquire",
			Type:     funcapi.FieldTypeString,
			Sort:     funcapi.FieldSortAscending,
			Sortable: false,
			Summary:  funcapi.FieldSummaryCount,
			Filter:   funcapi.FieldFilterMultiselect,
			Visible:  true,
		},
		Value: func(r *deadlockRowData) any { return r.waitResource },
	},
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "spid",
			Tooltip:   "MySQL thread/connection ID",
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
			Name:     "process_id",
			Tooltip:  "Transaction identifier from InnoDB",
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
	{
		ColumnMeta: funcapi.ColumnMeta{
			Name:      "ecid",
			Tooltip:   "Execution Context ID (SQL Server concept, not used in MySQL)",
			Type:      funcapi.FieldTypeInteger,
			Sort:      funcapi.FieldSortAscending,
			Sortable:  true,
			Summary:   funcapi.FieldSummaryCount,
			Filter:    funcapi.FieldFilterRange,
			Visible:   false, // SQL Server concept, not applicable to MySQL/MariaDB
			Transform: funcapi.FieldTransformNumber,
		},
		Value: func(r *deadlockRowData) any { return nil },
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
	router *router
}

func newFuncDeadlockInfo(r *router) *funcDeadlockInfo {
	return &funcDeadlockInfo{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcDeadlockInfo)(nil)

func (f *funcDeadlockInfo) MethodParams(ctx context.Context, method string) ([]funcapi.ParamConfig, error) {
	if f.router.cfg.deadlockInfoDisabled() {
		return nil, fmt.Errorf("deadlock-info function disabled in configuration")
	}
	return []funcapi.ParamConfig{}, nil
}

func (f *funcDeadlockInfo) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if _, err := f.router.deps.DB(); err != nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}
	queryCtx, cancel := context.WithTimeout(ctx, f.router.cfg.deadlockInfoTimeout())
	defer cancel()
	return f.collectData(queryCtx)
}

func (f *funcDeadlockInfo) Cleanup(ctx context.Context) {}

func (f *funcDeadlockInfo) collectData(ctx context.Context) *funcapi.FunctionResponse {
	if f.router.cfg.deadlockInfoDisabled() {
		return funcapi.UnavailableResponse("deadlock-info function has been disabled in configuration")
	}

	statusText, err := f.queryInnoDBStatus(ctx)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return f.buildResponse(504, "deadlock query timed out", nil)
		}
		if isMySQLPermissionError(err) {
			return f.buildResponse(
				403,
				"Deadlock info requires permission to run SHOW ENGINE INNODB STATUS. "+
					"Grant with: GRANT USAGE, REPLICATION CLIENT, PROCESS ON *.* TO 'netdata'@'%';",
				nil,
			)
		}
		f.router.log.Warningf("deadlock-info: query failed: %v", err)
		return f.buildResponse(500, fmt.Sprintf("deadlock query failed: %v", err), nil)
	}

	parseRes := parseInnoDBDeadlock(statusText, time.Now().UTC())
	if parseRes.parseErr != nil {
		f.router.log.Warningf("deadlock-info: parse failed: %v", parseRes.parseErr)
		return f.buildResponse(deadlockParseErrorStatus, "deadlock section could not be parsed", nil)
	}
	if !parseRes.found {
		return f.buildResponse(200, "no deadlock found in SHOW ENGINE INNODB STATUS", nil)
	}

	deadlockID := generateDeadlockID(parseRes.deadlockTime)
	rows := buildDeadlockRows(parseRes, deadlockID)
	if len(rows) == 0 {
		return f.buildResponse(200, "deadlock detected but no transactions could be parsed", nil)
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

func (f *funcDeadlockInfo) queryInnoDBStatus(ctx context.Context) (string, error) {
	qctx, cancel := context.WithTimeout(ctx, f.router.cfg.collectorTimeout())
	defer cancel()

	db, err := f.router.deps.DB()
	if err != nil {
		return "", err
	}

	var typ, name, status sql.NullString
	row := db.QueryRowContext(qctx, queryShowEngineInnoDBStatus)
	if err := row.Scan(&typ, &name, &status); err != nil {
		return "", err
	}
	if !status.Valid {
		return "", fmt.Errorf("innodb status response was empty")
	}
	return status.String, nil
}

type mysqlDeadlockTxn struct {
	txnNum       int
	threadID     string
	queryText    string
	lockMode     string
	lockStatus   string
	waitResource string
}

type mysqlDeadlockParseResult struct {
	found        bool
	deadlockTime time.Time
	victimTxnNum int
	transactions []*mysqlDeadlockTxn
	parseErr     error
}

func parseInnoDBDeadlock(status string, now time.Time) mysqlDeadlockParseResult {
	result := mysqlDeadlockParseResult{
		found:        false,
		deadlockTime: now.UTC(),
	}

	section, ok := extractDeadlockSection(status)
	if !ok {
		return result
	}
	result.found = true

	if ts, ok := parseDeadlockTimestamp(section); ok {
		result.deadlockTime = ts.UTC()
	}

	scanner := bufio.NewScanner(strings.NewReader(section))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	txnByNum := make(map[int]*mysqlDeadlockTxn)
	txnOrder := make([]int, 0, 4)

	currentTxnNum := 0
	currentSection := ""
	expectingQueryTxn := 0
	victimTxnNum := 0

	ensureTxn := func(num int) *mysqlDeadlockTxn {
		if txn, ok := txnByNum[num]; ok {
			return txn
		}
		txn := &mysqlDeadlockTxn{txnNum: num}
		txnByNum[num] = txn
		txnOrder = append(txnOrder, num)
		return txn
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if num, ok := parseDeadlockTxnHeader(line); ok {
			currentTxnNum = num
			currentSection = ""
			expectingQueryTxn = 0
			ensureTxn(num)
			continue
		}

		if num, sectionType, ok := parseDeadlockTxnSection(line); ok {
			currentTxnNum = num
			currentSection = sectionType
			expectingQueryTxn = 0
			ensureTxn(num)
			continue
		}

		if currentTxnNum != 0 {
			if isDeadlockWaitNoTxn(line) {
				currentSection = deadlockSectionWaiting
				expectingQueryTxn = 0
				ensureTxn(currentTxnNum)
				continue
			}
			if isDeadlockHoldsNoTxn(line) {
				currentSection = deadlockSectionHolds
				expectingQueryTxn = 0
				ensureTxn(currentTxnNum)
				continue
			}
		}

		if num, ok := parseDeadlockVictim(line); ok {
			victimTxnNum = num
			continue
		}

		if currentTxnNum == 0 {
			continue
		}

		txn := ensureTxn(currentTxnNum)

		if threadID, ok := parseDeadlockThreadID(line); ok {
			txn.threadID = threadID
			expectingQueryTxn = currentTxnNum
			continue
		}

		if expectingQueryTxn == currentTxnNum && txn.queryText == "" && isSQLStatementLine(line) {
			txn.queryText = line
			expectingQueryTxn = 0
			continue
		}

		if expectingQueryTxn == 0 && txn.queryText == "" && isSQLStatementLine(line) {
			txn.queryText = line
			continue
		}

		switch currentSection {
		case deadlockSectionWaiting:
			// WAITING must win even if HOLDS was seen first in the output.
			txn.lockStatus = "WAITING"
			if txn.waitResource == "" && isLockResourceLine(line) {
				txn.waitResource = strmutil.TruncateText(line, topQueriesMaxTextLength)
			}
			if mode, ok := parseDeadlockLockMode(line); ok {
				// WAITING lock mode should override any mode captured from HOLDS.
				txn.lockMode = mode
			}
		case deadlockSectionHolds:
			if txn.lockStatus == "" {
				txn.lockStatus = "GRANTED"
			}
			if txn.lockMode == "" {
				if mode, ok := parseDeadlockLockMode(line); ok {
					txn.lockMode = mode
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		result.parseErr = err
		return result
	}

	result.victimTxnNum = victimTxnNum
	result.transactions = make([]*mysqlDeadlockTxn, 0, len(txnOrder))
	for _, num := range txnOrder {
		txn := txnByNum[num]
		if txn == nil {
			continue
		}
		if txn.threadID == "" {
			txn.threadID = fmt.Sprintf("txn-%d", num)
		}
		if txn.lockStatus == "" {
			if num == victimTxnNum {
				txn.lockStatus = "WAITING"
			} else {
				txn.lockStatus = "GRANTED"
			}
		}
		result.transactions = append(result.transactions, txn)
	}

	if len(result.transactions) == 0 {
		result.parseErr = fmt.Errorf("deadlock section detected but no transactions could be parsed")
		return result
	}

	sort.Slice(result.transactions, func(i, j int) bool {
		return result.transactions[i].txnNum < result.transactions[j].txnNum
	})

	return result
}

func buildDeadlockRows(parseRes mysqlDeadlockParseResult, deadlockID string) []deadlockRowData {
	rows := make([]deadlockRowData, 0, len(parseRes.transactions))
	timestamp := parseRes.deadlockTime.UTC().Format(time.RFC3339Nano)

	for _, txn := range parseRes.transactions {
		if txn == nil {
			continue
		}

		processID := strings.TrimSpace(txn.threadID)
		if processID == "" {
			processID = fmt.Sprintf("txn-%d", txn.txnNum)
		}

		var spid any
		if id, err := strconv.Atoi(processID); err == nil {
			spid = id
		} else {
			spid = nil
		}

		isVictim := "false"
		if parseRes.victimTxnNum != 0 && txn.txnNum == parseRes.victimTxnNum {
			isVictim = "true"
		}

		queryText := strmutil.TruncateText(strings.TrimSpace(txn.queryText), topQueriesMaxTextLength)
		lockMode := formatLockMode(strings.TrimSpace(txn.lockMode))
		lockStatus := strings.TrimSpace(txn.lockStatus)
		waitResource := strmutil.TruncateText(strings.TrimSpace(txn.waitResource), topQueriesMaxTextLength)
		database := extractDeadlockDatabase(waitResource, queryText)

		var databaseValue any
		if database != "" {
			databaseValue = database
		}

		rows = append(rows, deadlockRowData{
			rowID:        fmt.Sprintf("%s:%s", deadlockID, processID),
			deadlockID:   deadlockID,
			timestamp:    timestamp,
			processID:    processID,
			spid:         spid,
			isVictim:     isVictim,
			queryText:    queryText,
			lockMode:     lockMode,
			lockStatus:   lockStatus,
			waitResource: waitResource,
			database:     databaseValue,
		})
	}

	return rows
}

func extractDeadlockSection(status string) (string, bool) {
	idx := reDeadlockHeader.FindStringIndex(status)
	if idx == nil {
		return "", false
	}
	return status[idx[0]:], true
}

func parseDeadlockTimestamp(section string) (time.Time, bool) {
	match := reDeadlockTS.FindString(section)
	if match == "" {
		return time.Time{}, false
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04:05", match, time.Local)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

func parseDeadlockTxnHeader(line string) (int, bool) {
	m := reDeadlockTxn.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseDeadlockTxnSection(line string) (int, string, bool) {
	if m := reDeadlockWait.FindStringSubmatch(line); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, "", false
		}
		return n, deadlockSectionWaiting, true
	}
	if m := reDeadlockHolds.FindStringSubmatch(line); len(m) == 2 {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, "", false
		}
		return n, deadlockSectionHolds, true
	}
	return 0, "", false
}

func isDeadlockWaitNoTxn(line string) bool {
	return reDeadlockWaitNoTxn.MatchString(line)
}

func isDeadlockHoldsNoTxn(line string) bool {
	return reDeadlockHoldsNoTxn.MatchString(line)
}

func parseDeadlockVictim(line string) (int, bool) {
	m := reDeadlockVictim.FindStringSubmatch(line)
	if len(m) != 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseDeadlockThreadID(line string) (string, bool) {
	m := reDeadlockThread.FindStringSubmatch(line)
	if len(m) != 2 {
		return "", false
	}
	return m[1], true
}

func parseDeadlockLockMode(line string) (string, bool) {
	m := reDeadlockMode.FindStringSubmatch(line)
	if len(m) != 2 {
		return "", false
	}
	return strings.ToUpper(m[1]), true
}

func isLikelyQueryLine(line string) bool {
	return isSQLStatementLine(line)
}

func isSQLStatementLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(upper, "LOCK WAIT") {
		return false
	}
	return reSQLStatement.MatchString(trimmed)
}

func isLockResourceLine(line string) bool {
	upper := strings.ToUpper(strings.TrimSpace(line))
	return strings.HasPrefix(upper, "RECORD LOCKS") || strings.HasPrefix(upper, "TABLE LOCK")
}

func extractDeadlockDatabase(waitResource, queryText string) string {
	if db := extractDatabaseFromLock(waitResource); db != "" {
		return db
	}
	if db := extractDatabaseFromQuery(queryText); db != "" {
		return db
	}
	return ""
}

func extractDatabaseFromLock(line string) string {
	m := reDeadlockTable.FindStringSubmatch(line)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func extractDatabaseFromQuery(queryText string) string {
	m := reQueryTableRef.FindStringSubmatch(queryText)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

func generateDeadlockID(t time.Time) string {
	if t.IsZero() {
		t = time.Now().UTC()
	}
	t = t.UTC()
	micros := t.Nanosecond() / 1000
	return t.Format("20060102150405") + fmt.Sprintf("%06d", micros)
}

func isMySQLPermissionError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		if mysqlErr.Number == 1045 || mysqlErr.Number == 1227 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "access denied") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "process privilege")
}

// formatLockMode converts InnoDB lock mode abbreviations to human-readable format.
func formatLockMode(mode string) string {
	names := map[string]string{
		"X":  "Exclusive",
		"S":  "Shared",
		"IX": "Intent Exclusive",
		"IS": "Intent Shared",
	}
	if name, ok := names[mode]; ok {
		return fmt.Sprintf("%s (%s)", name, mode)
	}
	return mode
}
