package zabbixpreproc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// csvToJSON converts CSV data to JSON.
func csvToJSON(value Value, paramStr string) (Value, error) {
	// paramStr format: "delimiter\nquote_character\nheader_row"
	// header_row: 1 = first row is header, 0 = no header (use column indices)
	params := strings.Split(paramStr, "\n")

	// Validate parameter count: must be exactly 3
	if len(params) < 2 {
		return Value{}, fmt.Errorf("csv to json requires delimiter, quote, and header parameters")
	}
	if len(params) < 3 {
		return Value{}, fmt.Errorf("csv to json requires header row parameter")
	}

	// Extract delimiter (UTF-8 multi-byte support)
	delim := ","
	delimExplicit := false // Track if delimiter was explicitly set
	if params[0] != "" {
		r, size := utf8.DecodeRuneInString(params[0])
		if size != len(params[0]) {
			// Multi-character delimiter not allowed
			return Value{}, fmt.Errorf("invalid delimiter: must be single character")
		}
		delim = string(r)
		delimExplicit = true
	}

	// Extract quote character (UTF-8 multi-byte support)
	quote := "\""
	if params[1] != "" {
		r, size := utf8.DecodeRuneInString(params[1])
		if size != len(params[1]) {
			// Multi-character quote not allowed
			return Value{}, fmt.Errorf("invalid quote character: must be single character")
		}
		quote = string(r)
	}

	// Extract header_row flag
	headerRow, err := strconv.Atoi(params[2])
	if err != nil {
		return Value{}, fmt.Errorf("invalid header row parameter")
	}
	hasHeader := (headerRow == 1)

	// Check for Sep=/SEP=/sEp= declaration (case-INSENSITIVE)
	// Sep= overrides the delimiter param for ALL rows
	data := value.Data
	lines := strings.Split(data, "\n")
	startLine := 0

	if len(lines) > 0 {
		firstLine := strings.TrimRight(lines[0], "\r") // Handle \r\n line breaks
		// Case-insensitive check for "sep="
		if len(firstLine) >= 4 && strings.ToLower(firstLine[:4]) == "sep=" {
			if len(firstLine) == 4 {
				// "sep=" with no character - invalid, treat as data
			} else {
				// Check if exactly one character after "sep="
				r, size := utf8.DecodeRuneInString(firstLine[4:])
				if size == len(firstLine[4:]) {
					// Valid single-char sep= line
					// Only override delimiter if it wasn't explicitly set in params
					if !delimExplicit {
						delim = string(r)
					}
					startLine = 1
					// Reconstruct data without sep= line
					data = strings.Join(lines[startLine:], "\n")
				}
				// If multi-char after "sep=", treat as data (don't skip)
			}
		}
	}

	// Check if input ends with newline (for empty row logic)
	endsWithNewline := strings.HasSuffix(value.Data, "\n") || strings.HasSuffix(value.Data, "\r\n")

	// Parse CSV with strict validation
	records, err := parseCSVStrict(data, delim, quote)
	if err != nil {
		return Value{}, err
	}

	// Empty input returns []
	if len(records) == 0 {
		return Value{Data: "[]", Type: ValueTypeStr}, nil
	}

	var jsonData interface{}

	if hasHeader {
		// First row is header
		if len(records) < 1 {
			// Only header line, no data
			return Value{Data: "[]", Type: ValueTypeStr}, nil
		}

		header := records[0]

		// Validate no duplicate column names
		seen := make(map[string]bool)
		for _, col := range header {
			if seen[col] {
				return Value{}, fmt.Errorf("duplicate column name: %s", col)
			}
			seen[col] = true
		}

		result := make([]map[string]string, 0)

		// Add data rows
		for i := 1; i < len(records); i++ {
			row := records[i]

			// Validate field count: data row can't have more fields than header
			if len(row) > len(header) {
				return Value{}, fmt.Errorf("data row has more fields than header")
			}

			obj := make(map[string]string)
			for j, val := range row {
				if j < len(header) {
					obj[header[j]] = val
				}
			}
			// Fill missing fields with empty strings
			for j := len(row); j < len(header); j++ {
				obj[header[j]] = ""
			}
			result = append(result, obj)
		}

		// Add empty trailing row if input ended with newline
		if endsWithNewline {
			emptyObj := make(map[string]string)
			for _, h := range header {
				emptyObj[h] = ""
			}
			result = append(result, emptyObj)
		}

		// Marshal with ordered keys to preserve header column order
		jsonData, err = marshalOrderedJSON(result, header)
		if err != nil {
			return Value{}, fmt.Errorf("failed to marshal csv to json: %w", err)
		}
	} else {
		// No header - use 1-based column indices as keys
		result := make([]map[string]string, 0)
		for _, row := range records {
			// Special case: single empty field becomes empty object
			if len(row) == 1 && row[0] == "" {
				result = append(result, make(map[string]string))
			} else {
				obj := make(map[string]string)
				for i, val := range row {
					// Use 1-based indexing
					obj[strconv.Itoa(i+1)] = val
				}
				result = append(result, obj)
			}
		}

		// Add empty trailing row if input ended with newline
		if endsWithNewline && len(result) > 0 {
			result = append(result, make(map[string]string))
		}

		jsonData = result
	}

	var jsonBytes []byte
	if str, ok := jsonData.(string); ok {
		jsonBytes = []byte(str)
	} else {
		jsonBytes, err = json.Marshal(jsonData)
		if err != nil {
			return Value{}, fmt.Errorf("failed to marshal csv to json: %w", err)
		}
	}

	return Value{Data: string(jsonBytes), Type: ValueTypeStr}, nil
}

// marshalOrderedJSON marshals array of maps with keys in header order.
func marshalOrderedJSON(data []map[string]string, header []string) (string, error) {
	var builder strings.Builder
	builder.WriteString("[")

	for i, obj := range data {
		if i > 0 {
			builder.WriteString(",")
		}
		// Delegate single object marshaling to avoid code duplication
		jsonObj, err := marshalOrderedJSONSingle(obj, header)
		if err != nil {
			return "", err
		}
		builder.WriteString(jsonObj)
	}

	builder.WriteString("]")
	return builder.String(), nil
}

// parseCSVStrict parses CSV data with strict Zabbix-compatible validation.
//
// This custom parser implements Zabbix's specific CSV parsing rules, which differ
// from encoding/csv in several ways:
//   - Supports multi-byte UTF-8 delimiters and quote characters (not just single runes)
//   - Special mode when delimiter == quote (disables quoting behavior entirely)
//   - Strict validation: rejects \n\r line breaks, unclosed quotes, invalid characters after closing quotes
//   - UTF-8 validation: returns error on invalid UTF-8 sequences
//
// Parameters:
//   - data: Raw CSV string data to parse
//   - delimiter: Field delimiter (can be multi-byte UTF-8 string like "€")
//   - quote: Quote character (can be multi-byte UTF-8 string like "§")
//
// Returns:
//   - [][]string: Parsed CSV records (array of rows, each row is array of fields)
//   - error: Parsing error if data is invalid
//
// This function cannot be replaced with encoding/csv.Reader due to the multi-byte
// delimiter/quote support and special delimiter==quote handling required by Zabbix.

// csvParseState tracks the state during CSV parsing
type csvParseState struct {
	records       [][]string
	currentRecord []string
	currentField  strings.Builder
	inQuotes      bool
	fieldStart    bool
	data          string
	delimiter     string
	quote         string
}

// finishField completes the current field and adds it to the current record
func (s *csvParseState) finishField() {
	s.currentRecord = append(s.currentRecord, s.currentField.String())
	s.currentField.Reset()
	s.fieldStart = true
}

// finishRecord completes the current record and adds it to results
func (s *csvParseState) finishRecord() {
	if len(s.currentRecord) > 0 {
		s.records = append(s.records, s.currentRecord)
	}
	s.currentRecord = nil
	s.fieldStart = true
}

// handleQuoteChar processes a quote character at position i
// Returns new position and whether to continue main loop
func (s *csvParseState) handleQuoteChar(i int) (int, bool, error) {
	if s.inQuotes {
		// Check if it's an escaped quote (doubled)
		if strings.HasPrefix(s.data[i+len(s.quote):], s.quote) {
			// Escaped quote - add one quote to field
			s.currentField.WriteString(s.quote)
			s.fieldStart = false
			return i + len(s.quote)*2, true, nil
		}

		// End of quoted field
		s.inQuotes = false
		i += len(s.quote)

		// After closing quote, MUST be delimiter or newline
		if i < len(s.data) {
			nextChar := s.data[i]
			if nextChar != '\n' && nextChar != '\r' && !strings.HasPrefix(s.data[i:], s.delimiter) {
				return 0, false, fmt.Errorf("character after closing quote")
			}
		}
		s.fieldStart = false
		return i, true, nil
	}

	if s.fieldStart {
		// Start of quoted field (only at field start)
		s.inQuotes = true
		s.fieldStart = false
		return i + len(s.quote), true, nil
	}

	// Quote in middle of unquoted field - treat as literal
	s.currentField.WriteString(s.quote)
	s.fieldStart = false
	return i + len(s.quote), true, nil
}

// handleNewlineChar processes a newline character at position i (only outside quotes)
// Returns new position and whether to continue main loop
func (s *csvParseState) handleNewlineChar(i int) (int, bool, error) {
	if s.data[i] == '\n' {
		// Check for unsupported \n\r line break
		if i+1 < len(s.data) && s.data[i+1] == '\r' {
			return 0, false, fmt.Errorf("unsupported line break")
		}

		// End of record
		s.currentRecord = append(s.currentRecord, s.currentField.String())
		s.currentField.Reset()
		s.finishRecord()
		return i + 1, true, nil
	}

	if s.data[i] == '\r' {
		// Handle \r\n or just \r
		if i+1 < len(s.data) && s.data[i+1] == '\n' {
			// \r\n - end of record
			s.currentRecord = append(s.currentRecord, s.currentField.String())
			s.currentField.Reset()
			s.finishRecord()
			return i + 2, true, nil
		}

		// Just \r - treat as newline
		s.currentRecord = append(s.currentRecord, s.currentField.String())
		s.currentField.Reset()
		s.finishRecord()
		return i + 1, true, nil
	}

	return i, false, nil
}

func parseCSVStrict(data string, delimiter string, quote string) ([][]string, error) {
	// Special case: when delimiter == quote, disable quoting behavior
	if delimiter == quote {
		return parseCSVNoQuotes(data, delimiter)
	}

	// Initialize parsing state
	state := &csvParseState{
		data:       data,
		delimiter:  delimiter,
		quote:      quote,
		fieldStart: true,
	}

	i := 0
	for i < len(data) {
		// Check for quote character at current position
		if strings.HasPrefix(data[i:], quote) {
			newPos, shouldContinue, err := state.handleQuoteChar(i)
			if err != nil {
				return nil, err
			}
			if shouldContinue {
				i = newPos
				continue
			}
		}

		// Check for delimiter (only outside quotes)
		if !state.inQuotes && strings.HasPrefix(data[i:], delimiter) {
			state.finishField()
			i += len(delimiter)
			continue
		}

		// Check for newline (only outside quotes)
		if !state.inQuotes {
			newPos, shouldContinue, err := state.handleNewlineChar(i)
			if err != nil {
				return nil, err
			}
			if shouldContinue {
				i = newPos
				continue
			}
		}

		// Regular character - add to current field
		r, size := utf8.DecodeRuneInString(data[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("invalid UTF-8 character at position %d", i)
		}
		state.currentField.WriteRune(r)
		i += size
		state.fieldStart = false
	}

	// Handle last field and record
	if state.inQuotes {
		return nil, fmt.Errorf("unclosed quoted field")
	}

	// Add last field to current record
	if state.currentField.Len() > 0 || len(state.currentRecord) > 0 {
		state.currentRecord = append(state.currentRecord, state.currentField.String())
	}

	// Add last record if not empty
	if len(state.currentRecord) > 0 {
		state.records = append(state.records, state.currentRecord)
	}

	return state.records, nil
}

// parseCSVNoQuotes parses CSV when delimiter == quote (no quoting behavior).
func parseCSVNoQuotes(data string, delimiter string) ([][]string, error) {
	var records [][]string
	var currentRecord []string
	var currentField strings.Builder

	i := 0

	for i < len(data) {
		// Check for delimiter
		if strings.HasPrefix(data[i:], delimiter) {
			currentRecord = append(currentRecord, currentField.String())
			currentField.Reset()
			i += len(delimiter)
			continue
		}

		// Check for newline
		if data[i] == '\n' {
			// Check for \n\r
			if i+1 < len(data) && data[i+1] == '\r' {
				return nil, fmt.Errorf("unsupported line break")
			}

			// End of record
			currentRecord = append(currentRecord, currentField.String())
			currentField.Reset()
			if len(currentRecord) > 0 {
				records = append(records, currentRecord)
			}
			currentRecord = nil
			i++
			continue
		}
		if data[i] == '\r' {
			// Handle \r\n or just \r
			if i+1 < len(data) && data[i+1] == '\n' {
				currentRecord = append(currentRecord, currentField.String())
				currentField.Reset()
				if len(currentRecord) > 0 {
					records = append(records, currentRecord)
				}
				currentRecord = nil
				i += 2
				continue
			} else {
				currentRecord = append(currentRecord, currentField.String())
				currentField.Reset()
				if len(currentRecord) > 0 {
					records = append(records, currentRecord)
				}
				currentRecord = nil
				i++
				continue
			}
		}

		// Regular character
		r, size := utf8.DecodeRuneInString(data[i:])
		if r == utf8.RuneError && size == 1 {
			return nil, fmt.Errorf("invalid UTF-8 character at position %d", i)
		}
		currentField.WriteRune(r)
		i += size
	}

	// Handle last field and record
	if currentField.Len() > 0 || len(currentRecord) > 0 {
		currentRecord = append(currentRecord, currentField.String())
	}
	if len(currentRecord) > 0 {
		records = append(records, currentRecord)
	}

	return records, nil
}

// csvToJSONMulti converts CSV data to multiple Result metrics
// Each CSV row becomes a separate Result.Metric with row index label
func csvToJSONMulti(value Value, paramStr string) (Result, error) {
	// paramStr format: "delimiter\nquote_character\nheader_row"
	// header_row: 1 = first row is header, 0 = no header (use column indices)
	params := strings.Split(paramStr, "\n")

	// Validate parameter count: must be exactly 3
	if len(params) < 2 {
		err := fmt.Errorf("csv to json requires delimiter, quote, and header parameters")
		return Result{Error: err}, err
	}
	if len(params) < 3 {
		err := fmt.Errorf("csv to json requires header row parameter")
		return Result{Error: err}, err
	}

	// Extract delimiter (UTF-8 multi-byte support)
	delim := ","
	delimExplicit := false // Track if delimiter was explicitly set
	if params[0] != "" {
		r, size := utf8.DecodeRuneInString(params[0])
		if size != len(params[0]) {
			// Multi-character delimiter not allowed
			err := fmt.Errorf("invalid delimiter: must be single character")
			return Result{Error: err}, err
		}
		delim = string(r)
		delimExplicit = true
	}

	// Extract quote character (UTF-8 multi-byte support)
	quote := "\""
	if params[1] != "" {
		r, size := utf8.DecodeRuneInString(params[1])
		if size != len(params[1]) {
			// Multi-character quote not allowed
			err := fmt.Errorf("invalid quote character: must be single character")
			return Result{Error: err}, err
		}
		quote = string(r)
	}

	// Extract header_row flag
	headerRow, err := strconv.Atoi(params[2])
	if err != nil {
		err = fmt.Errorf("invalid header row parameter")
		return Result{Error: err}, err
	}
	hasHeader := (headerRow == 1)

	// Check for Sep=/SEP=/sEp= declaration (case-INSENSITIVE)
	// Sep= overrides the delimiter param for ALL rows
	data := value.Data
	lines := strings.Split(data, "\n")
	startLine := 0

	if len(lines) > 0 {
		firstLine := strings.TrimRight(lines[0], "\r") // Handle \r\n line breaks
		// Case-insensitive check for "sep="
		if len(firstLine) >= 4 && strings.ToLower(firstLine[:4]) == "sep=" {
			if len(firstLine) == 4 {
				// "sep=" with no character - invalid, treat as data
			} else {
				// Check if exactly one character after "sep="
				r, size := utf8.DecodeRuneInString(firstLine[4:])
				if size == len(firstLine[4:]) {
					// Valid single-char sep= line
					// Only override delimiter if it wasn't explicitly set in params
					if !delimExplicit {
						delim = string(r)
					}
					startLine = 1
					// Reconstruct data without sep= line
					data = strings.Join(lines[startLine:], "\n")
				}
				// If multi-char after "sep=", treat as data (don't skip)
			}
		}
	}

	// Check if input ends with newline (for empty row logic)
	endsWithNewline := strings.HasSuffix(value.Data, "\n") || strings.HasSuffix(value.Data, "\r\n")

	// Parse CSV with strict validation
	records, err := parseCSVStrict(data, delim, quote)
	if err != nil {
		return Result{Error: err}, err
	}

	// Empty input returns empty metrics array
	if len(records) == 0 {
		return Result{Metrics: []Metric{}}, nil
	}

	var metrics []Metric

	if hasHeader {
		// First row is header
		if len(records) < 1 {
			// Only header line, no data
			return Result{Metrics: []Metric{}}, nil
		}

		header := records[0]

		// Validate no duplicate column names
		seen := make(map[string]bool)
		for _, col := range header {
			if seen[col] {
				err := fmt.Errorf("duplicate column name: %s", col)
				return Result{Error: err}, err
			}
			seen[col] = true
		}

		// Convert each data row to a metric
		for i := 1; i < len(records); i++ {
			row := records[i]

			// Validate field count: data row can't have more fields than header
			if len(row) > len(header) {
				err := fmt.Errorf("data row has more fields than header")
				return Result{Error: err}, err
			}

			obj := make(map[string]string)
			for j, val := range row {
				if j < len(header) {
					obj[header[j]] = val
				}
			}
			// Fill missing fields with empty strings
			for j := len(row); j < len(header); j++ {
				obj[header[j]] = ""
			}

			// Marshal row to JSON object string
			jsonBytes, err := marshalOrderedJSONSingle(obj, header)
			if err != nil {
				return Result{Error: err}, err
			}

			metrics = append(metrics, Metric{
				Name:   "csv_row",
				Value:  jsonBytes,
				Type:   ValueTypeStr,
				Labels: map[string]string{"row": strconv.Itoa(i)}, // 1-based row number (excluding header)
			})
		}

		// Add empty trailing row if input ended with newline
		if endsWithNewline {
			emptyObj := make(map[string]string)
			for _, h := range header {
				emptyObj[h] = ""
			}
			jsonBytes, err := marshalOrderedJSONSingle(emptyObj, header)
			if err != nil {
				return Result{Error: err}, err
			}
			metrics = append(metrics, Metric{
				Name:   "csv_row",
				Value:  jsonBytes,
				Type:   ValueTypeStr,
				Labels: map[string]string{"row": strconv.Itoa(len(metrics) + 1)},
			})
		}
	} else {
		// No header - use 1-based column indices as keys
		for i, row := range records {
			var obj map[string]string

			// Special case: single empty field becomes empty object
			if len(row) == 1 && row[0] == "" {
				obj = make(map[string]string)
			} else {
				obj = make(map[string]string)
				for j, val := range row {
					// Use 1-based indexing
					obj[strconv.Itoa(j+1)] = val
				}
			}

			// Marshal row to JSON object string
			jsonBytes, err := json.Marshal(obj)
			if err != nil {
				return Result{Error: err}, err
			}

			metrics = append(metrics, Metric{
				Name:   "csv_row",
				Value:  string(jsonBytes),
				Type:   ValueTypeStr,
				Labels: map[string]string{"row": strconv.Itoa(i + 1)}, // 1-based row number
			})
		}

		// Add empty trailing row if input ended with newline
		if endsWithNewline && len(metrics) > 0 {
			emptyJSON, _ := json.Marshal(make(map[string]string))
			metrics = append(metrics, Metric{
				Name:   "csv_row",
				Value:  string(emptyJSON),
				Type:   ValueTypeStr,
				Labels: map[string]string{"row": strconv.Itoa(len(metrics) + 1)},
			})
		}
	}

	return Result{Metrics: metrics}, nil
}

// marshalOrderedJSONSingle marshals a single map with keys in header order.
func marshalOrderedJSONSingle(obj map[string]string, header []string) (string, error) {
	var builder strings.Builder
	builder.WriteString("{")
	first := true
	for _, key := range header {
		val := obj[key]
		if !first {
			builder.WriteString(",")
		}
		first = false
		// Properly escape JSON key and value
		keyJSON, _ := json.Marshal(key)
		valJSON, _ := json.Marshal(val)
		builder.Write(keyJSON)
		builder.WriteString(":")
		builder.Write(valJSON)
	}
	builder.WriteString("}")
	return builder.String(), nil
}
