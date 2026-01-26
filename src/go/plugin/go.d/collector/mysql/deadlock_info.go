// SPDX-License-Identifier: GPL-3.0-or-later

package mysql

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

const deadlockInfoMethodID = "deadlock-info"

const (
	deadlockSectionWaiting = "waiting"
	deadlockSectionHolds   = "holds"
)

var (
	reDeadlockHeader = regexp.MustCompile(`LATEST DETECTED DEADLOCK`)
	reDeadlockTxn    = regexp.MustCompile(`^\*\*\* \((\d+)\) TRANSACTION:`)
	reDeadlockWait   = regexp.MustCompile(`^\*\*\* \((\d+)\) WAITING FOR THIS LOCK TO BE GRANTED:`)
	reDeadlockHolds  = regexp.MustCompile(`^\*\*\* \((\d+)\) HOLDS THE LOCK\(S\):`)
	reDeadlockVictim = regexp.MustCompile(`^\*\*\* WE ROLL BACK TRANSACTION \((\d+)\)`)
	reDeadlockThread = regexp.MustCompile(`MySQL thread id (\d+)`)
	reDeadlockMode   = regexp.MustCompile(`(?i)lock[_ ]mode\s+([A-Z0-9_-]+)`)
	reDeadlockTS     = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\b`)
)

const (
	deadlockInfoHelp         = "Latest detected deadlock from SHOW ENGINE INNODB STATUS. WARNING: query text may include unmasked sensitive literals; restrict dashboard access."
	deadlockParseErrorStatus = 561
)

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
		if err := f.router.collector.openConnection(); err != nil {
			return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
		}
	}
	return f.router.collector.collectDeadlockInfo(ctx)
}

func (f *funcDeadlockInfo) Cleanup(ctx context.Context) {}

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
				"To enable, set deadlock_info_function_enabled: true in the MySQL collector config.",
		}
	}

	statusText, err := c.queryInnoDBStatus(ctx)
	if err != nil {
		if isMySQLPermissionError(err) {
			return c.deadlockInfoResponse(
				403,
				"Deadlock info requires permission to run SHOW ENGINE INNODB STATUS. "+
					"Grant with: GRANT USAGE, REPLICATION CLIENT, PROCESS ON *.* TO 'netdata'@'%';",
				nil,
			)
		}
		c.Warningf("deadlock-info: query failed: %v", err)
		return c.deadlockInfoResponse(500, fmt.Sprintf("deadlock query failed: %v", err), nil)
	}

	parseRes := parseInnoDBDeadlock(statusText, time.Now().UTC())
	if parseRes.parseErr != nil {
		c.Warningf("deadlock-info: parse failed: %v", parseRes.parseErr)
		return c.deadlockInfoResponse(deadlockParseErrorStatus, "deadlock section could not be parsed", nil)
	}
	if !parseRes.found {
		return c.deadlockInfoResponse(200, "no deadlock found in SHOW ENGINE INNODB STATUS", nil)
	}

	deadlockID := generateDeadlockID(parseRes.deadlockTime)
	rows := buildDeadlockRows(parseRes, deadlockID)
	if len(rows) == 0 {
		return c.deadlockInfoResponse(200, "deadlock detected but no transactions could be parsed", nil)
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

func (c *Collector) queryInnoDBStatus(ctx context.Context) (string, error) {
	qctx, cancel := context.WithTimeout(ctx, c.Timeout.Duration())
	defer cancel()

	var typ, name, status sql.NullString
	if err := c.db.QueryRowContext(qctx, queryShowEngineInnoDBStatus).Scan(&typ, &name, &status); err != nil {
		return "", err
	}
	if !status.Valid {
		return "", fmt.Errorf("innodb status response was empty")
	}
	return status.String, nil
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

	columns := map[string]any{
		"row_id": funcapi.Column{
			Index:         deadlockIdxRowID,
			Name:          "Row ID",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
			Sticky:        false,
			Summary:       summaryCount,
			Filter:        filterMulti,
			FullWidth:     false,
			Wrap:          false,
			UniqueKey:     true,
			Visible:       false,
			ValueOptions: funcapi.ValueOptions{
				Transform:     trNone,
				DecimalPoints: 0,
			},
		}.BuildColumn(),
		"deadlock_id": funcapi.Column{
			Index:         deadlockIdxDeadlockID,
			Name:          "Deadlock ID",
			Type:          ftString,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
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
			Visualization: visValue,
			Sort:          sortDesc,
			Sortable:      true,
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
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
		"spid": funcapi.Column{
			Index:         deadlockIdxSpid,
			Name:          "Connection ID",
			Type:          ftInteger,
			Visualization: visValue,
			Sort:          sortAsc,
			Sortable:      true,
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
			Sticky:        true,
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
			Summary:       summaryCount,
			Filter:        filterMulti,
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
			Summary:       summaryCount,
			Filter:        filterMulti,
			Visible:       true,
			ValueOptions: funcapi.ValueOptions{
				Transform: trNone,
			},
		}.BuildColumn(),
	}
	return columns
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

		if expectingQueryTxn == currentTxnNum && txn.queryText == "" && isLikelyQueryLine(line) {
			txn.queryText = line
			expectingQueryTxn = 0
			continue
		}

		switch currentSection {
		case deadlockSectionWaiting:
			// WAITING must win even if HOLDS was seen first in the output.
			txn.lockStatus = "WAITING"
			if txn.waitResource == "" && isLockResourceLine(line) {
				txn.waitResource = strmutil.TruncateText(line, maxQueryTextLength)
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

func buildDeadlockRows(parseRes mysqlDeadlockParseResult, deadlockID string) [][]any {
	rows := make([][]any, 0, len(parseRes.transactions))
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

		queryText := strmutil.TruncateText(strings.TrimSpace(txn.queryText), maxQueryTextLength)
		lockMode := strings.TrimSpace(txn.lockMode)
		lockStatus := strings.TrimSpace(txn.lockStatus)
		waitResource := strmutil.TruncateText(strings.TrimSpace(txn.waitResource), maxQueryTextLength)

		row := make([]any, deadlockColumnCount)
		row[deadlockIdxRowID] = fmt.Sprintf("%s:%s", deadlockID, processID)
		row[deadlockIdxDeadlockID] = deadlockID
		row[deadlockIdxTimestamp] = timestamp
		row[deadlockIdxProcessID] = processID
		row[deadlockIdxSpid] = spid
		row[deadlockIdxEcid] = nil
		row[deadlockIdxIsVictim] = isVictim
		row[deadlockIdxQueryText] = queryText
		row[deadlockIdxLockMode] = lockMode
		row[deadlockIdxLockStatus] = lockStatus
		row[deadlockIdxWaitResource] = waitResource
		row[deadlockIdxDatabase] = nil
		rows = append(rows, row)
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
	upper := strings.ToUpper(strings.TrimSpace(line))
	if upper == "" {
		return false
	}
	if strings.HasPrefix(upper, "***") {
		return false
	}
	if strings.HasPrefix(upper, "RECORD LOCKS") || strings.HasPrefix(upper, "TABLE LOCK") {
		return false
	}
	return true
}

func isLockResourceLine(line string) bool {
	upper := strings.ToUpper(strings.TrimSpace(line))
	return strings.HasPrefix(upper, "RECORD LOCKS") || strings.HasPrefix(upper, "TABLE LOCK")
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
