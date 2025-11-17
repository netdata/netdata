package zabbixpreproc

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// valueInput represents the input value in a test case
type valueInput struct {
	ValueType string      `yaml:"value_type"`
	Time      string      `yaml:"time"`
	Data      interface{} `yaml:"data"`
}

// stepInput represents the preprocessing step in a test case
type stepInput struct {
	Type              string `yaml:"type"`
	Params            string `yaml:"params"`
	ErrorHandler      string `yaml:"error_handler"`
	ErrorHandlerParam string `yaml:"error_handler_params"`
}

// testOutput represents the expected output in a test case
type testOutput struct {
	Return string      `yaml:"return"`
	Value  interface{} `yaml:"value"`
	Error  string      `yaml:"error"`
}

// testInput represents the input section of a test case
type testInputSection struct {
	Value        valueInput  `yaml:"value"`
	Error        *valueInput `yaml:"error"`         // Input is an error instead of a value
	HistoryValue *valueInput `yaml:"history_value"` // Deprecated field
	History      *valueInput `yaml:"history"`       // Actual field used in tests
	Step         stepInput   `yaml:"step"`
}

// TestCase represents a single Zabbix preprocessing test case.
type TestCase struct {
	Name string
	In   testInputSection
	Out  testOutput
}

// LoadTestCases loads Zabbix YAML test cases from file.
func LoadTestCases(filename string) ([]TestCase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read test file: %w", err)
	}

	// Split YAML documents - split on "---" at start of line
	docStr := string(data)
	// Handle both with and without leading ---
	if strings.HasPrefix(docStr, "---") {
		docStr = docStr[3:] // Remove leading ---
	}

	docs := strings.Split(docStr, "\n---")
	var testCases []TestCase

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var rawTC struct {
			TestCase string           `yaml:"test case"`
			In       testInputSection `yaml:"in"`
			Out      testOutput       `yaml:"out"`
		}

		if err := yaml.Unmarshal([]byte(doc), &rawTC); err != nil {
			// Log warning for unparseable YAML (don't silently skip)
			fmt.Fprintf(os.Stderr, "WARNING: Failed to parse YAML document in test file: %v\n", err)
			continue
		}

		if rawTC.TestCase == "" {
			continue
		}

		testCases = append(testCases, TestCase{
			Name: rawTC.TestCase,
			In:   rawTC.In,
			Out:  rawTC.Out,
		})
	}

	return testCases, nil
}

// LoadXPathTestCases loads XPath-specific test cases and converts to standard format
func LoadXPathTestCases(filename string) ([]TestCase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read XPath test file: %w", err)
	}

	// Split YAML documents
	docStr := string(data)
	if strings.HasPrefix(docStr, "---") {
		docStr = docStr[3:]
	}

	docs := strings.Split(docStr, "\n---")
	var testCases []TestCase

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var rawTC struct {
			TestCase string `yaml:"test case"`
			In       struct {
				XML   string `yaml:"xml"`
				XPath string `yaml:"xpath"`
			} `yaml:"in"`
			Out struct {
				Result string `yaml:"result"`
				Return string `yaml:"return"`
			} `yaml:"out"`
		}

		if err := yaml.Unmarshal([]byte(doc), &rawTC); err != nil {
			// Log warning for unparseable YAML (don't silently skip)
			fmt.Fprintf(os.Stderr, "WARNING: Failed to parse XPath test YAML: %v\n", err)
			continue
		}

		if rawTC.TestCase == "" {
			continue
		}

		// Convert to standard format
		tc := TestCase{
			Name: rawTC.TestCase,
			In: testInputSection{
				Value: valueInput{
					ValueType: "ITEM_VALUE_TYPE_STR",
					Data:      rawTC.In.XML,
				},
				Step: stepInput{
					Type:   "ZBX_PREPROC_XPATH",
					Params: rawTC.In.XPath,
				},
			},
			Out: testOutput{
				Return: rawTC.Out.Return,
				Value:  rawTC.Out.Result,
			},
		}

		testCases = append(testCases, tc)
	}

	return testCases, nil
}

// LoadCSVTestCases loads CSV-specific test cases and converts to standard format
func LoadCSVTestCases(filename string) ([]TestCase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV test file: %w", err)
	}

	// Split YAML documents
	docStr := string(data)
	if strings.HasPrefix(docStr, "---") {
		docStr = docStr[3:]
	}

	docs := strings.Split(docStr, "\n---")
	var testCases []TestCase

	for _, doc := range docs {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var rawTC struct {
			TestCase string `yaml:"test case"`
			In       struct {
				CSV    string `yaml:"csv"`
				Params string `yaml:"params"`
			} `yaml:"in"`
			Out struct {
				Result string `yaml:"result"`
				Return string `yaml:"return"`
			} `yaml:"out"`
		}

		if err := yaml.Unmarshal([]byte(doc), &rawTC); err != nil {
			// Log warning for unparseable YAML (don't silently skip)
			fmt.Fprintf(os.Stderr, "WARNING: Failed to parse CSV test YAML: %v\n", err)
			continue
		}

		if rawTC.TestCase == "" {
			continue
		}

		// Convert to standard format
		tc := TestCase{
			Name: rawTC.TestCase,
			In: testInputSection{
				Value: valueInput{
					ValueType: "ITEM_VALUE_TYPE_STR",
					Data:      rawTC.In.CSV,
				},
				Step: stepInput{
					Type:   "ZBX_PREPROC_CSV_TO_JSON",
					Params: rawTC.In.Params,
				},
			},
			Out: testOutput{
				Return: rawTC.Out.Return,
				Value:  rawTC.Out.Result,
			},
		}

		testCases = append(testCases, tc)
	}

	return testCases, nil
}

// LoadAllTestCases loads all official Zabbix test suites
func LoadAllTestCases() ([]TestCase, error) {
	var allTests []TestCase

	// Load main test suite
	mainTests, err := LoadTestCases("testdata/zbx_item_preproc.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load main test suite: %w", err)
	}
	allTests = append(allTests, mainTests...)

	// Load XPath test suite
	xpathTests, err := LoadXPathTestCases("testdata/item_preproc_xpath.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load XPath test suite: %w", err)
	}
	allTests = append(allTests, xpathTests...)

	// Load CSV test suite
	csvTests, err := LoadCSVTestCases("testdata/item_preproc_csv_to_json.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to load CSV test suite: %w", err)
	}
	allTests = append(allTests, csvTests...)

	return allTests, nil
}

// StepTypeFromString converts Zabbix step type string to StepType.
func StepTypeFromString(s string) (StepType, error) {
	switch s {
	case "ZBX_PREPROC_MULTIPLIER":
		return StepTypeMultiplier, nil
	case "ZBX_PREPROC_TRIM":
		return StepTypeTrim, nil
	case "ZBX_PREPROC_RTRIM":
		return StepTypeRTrim, nil
	case "ZBX_PREPROC_LTRIM":
		return StepTypeLTrim, nil
	case "ZBX_PREPROC_REGSUB":
		return StepTypeRegexSubstitution, nil
	case "ZBX_PREPROC_BOOL2DEC":
		return StepTypeBool2Dec, nil
	case "ZBX_PREPROC_OCT2DEC":
		return StepTypeOct2Dec, nil
	case "ZBX_PREPROC_HEX2DEC":
		return StepTypeHex2Dec, nil
	case "ZBX_PREPROC_DELTA_VALUE":
		return StepTypeDeltaValue, nil
	case "ZBX_PREPROC_DELTA_SPEED":
		return StepTypeDeltaSpeed, nil
	case "ZBX_PREPROC_XPATH":
		return StepTypeXPath, nil
	case "ZBX_PREPROC_JSONPATH":
		return StepTypeJSONPath, nil
	case "ZBX_PREPROC_VALIDATE_RANGE":
		return StepTypeValidateRange, nil
	case "ZBX_PREPROC_VALIDATE_REGEX":
		return StepTypeValidateRegex, nil
	case "ZBX_PREPROC_VALIDATE_NOT_REGEX":
		return StepTypeValidateNotRegex, nil
	case "ZBX_PREPROC_ERROR_FIELD_JSON":
		return StepTypeErrorFieldJSON, nil
	case "ZBX_PREPROC_ERROR_FIELD_XML":
		return StepTypeErrorFieldXML, nil
	case "ZBX_PREPROC_ERROR_FIELD_REGEX":
		return StepTypeErrorFieldRegex, nil
	case "ZBX_PREPROC_THROTTLE_VALUE":
		return StepTypeThrottleValue, nil
	case "ZBX_PREPROC_THROTTLE_TIMED_VALUE":
		return StepTypeThrottleTimedValue, nil
	case "ZBX_PREPROC_PROMETHEUS_PATTERN":
		return StepTypePrometheusPattern, nil
	case "ZBX_PREPROC_PROMETHEUS_TO_JSON":
		return StepTypePrometheusToJSON, nil
	case "ZBX_PREPROC_CSV_TO_JSON":
		return StepTypeCSVToJSON, nil
	case "ZBX_PREPROC_STR_REPLACE":
		return StepTypeStringReplace, nil
	case "ZBX_PREPROC_SNMP_WALK_VALUE":
		return StepTypeSNMPWalkValue, nil
	case "ZBX_PREPROC_SCRIPT":
		return StepTypeJavaScript, nil
	case "ZBX_PREPROC_VALIDATE_NOT_SUPPORTED":
		return StepTypeValidateNotSupported, nil
	case "ZBX_PREPROC_XML_TO_JSON":
		return StepTypeXMLToJSON, nil
	case "ZBX_PREPROC_SNMP_GET_VALUE":
		return StepTypeSNMPGetValue, nil
	case "ZBX_PREPROC_SNMP_WALK_TO_JSON":
		return StepTypeSNMPWalkToJSON, nil
	default:
		return -1, fmt.Errorf("unknown step type: %s", s)
	}
}

// ValueFromTestInput converts test input to Value.
func ValueFromTestInput(tc TestCase) (Value, error) {
	// Check if this is an error input (from a previous step failure)
	if tc.In.Error != nil {
		data := ""
		if tc.In.Error.Data != nil {
			data = fmt.Sprint(tc.In.Error.Data)
		}

		timestamp := time.Now()
		if tc.In.Error.Time != "" {
			t, err := time.Parse("2006-01-02 15:04:05 -07:00", tc.In.Error.Time)
			if err == nil {
				timestamp = t
			}
		}

		return Value{
			Data:      data,
			Type:      ValueTypeStr, // Errors are always strings
			Timestamp: timestamp,
			IsError:   true, // Mark as error input
		}, nil
	}

	// Normal value input
	vt, err := parseValueType(tc.In.Value.ValueType)
	if err != nil {
		return Value{}, err
	}

	data := ""
	if tc.In.Value.Data != nil {
		data = fmt.Sprint(tc.In.Value.Data)
	}

	timestamp := time.Now()
	if tc.In.Value.Time != "" {
		// Parse time in format: 2017-10-29 03:15:00 +03:00
		t, err := time.Parse("2006-01-02 15:04:05 -07:00", tc.In.Value.Time)
		if err == nil {
			timestamp = t
		}
	}

	return Value{
		Data:      data,
		Type:      vt,
		Timestamp: timestamp,
		IsError:   false, // Normal value, not an error
	}, nil
}

// HistoryValueFromTestInput converts history value from test input to Value
func HistoryValueFromTestInput(tc TestCase) *Value {
	// Check both possible field names
	histValue := tc.In.HistoryValue
	if histValue == nil {
		histValue = tc.In.History
	}
	if histValue == nil {
		return nil
	}

	data := ""
	if histValue.Data != nil {
		data = fmt.Sprint(histValue.Data)
	}

	timestamp := time.Now()
	if histValue.Time != "" {
		t, err := time.Parse("2006-01-02 15:04:05 -07:00", histValue.Time)
		if err == nil {
			timestamp = t
		}
	}

	return &Value{
		Data:      data,
		Type:      ValueTypeStr,
		Timestamp: timestamp,
	}
}

// StepFromTestInput converts test input to Step.
func StepFromTestInput(tc TestCase) (Step, error) {
	stepType, err := StepTypeFromString(tc.In.Step.Type)
	if err != nil {
		return Step{}, err
	}

	// Parse error handler if provided
	errorHandler := ErrorHandler{Action: ErrorActionDefault}
	if tc.In.Step.ErrorHandler != "" {
		errorHandler = parseErrorHandler(tc.In.Step.ErrorHandler, tc.In.Step.ErrorHandlerParam)
	}

	return Step{
		Type:         stepType,
		Params:       tc.In.Step.Params,
		ErrorHandler: errorHandler,
	}, nil
}

// parseErrorHandler converts Zabbix error handler string to ErrorHandler
func parseErrorHandler(handlerStr, paramStr string) ErrorHandler {
	switch handlerStr {
	case "ZBX_PREPROC_FAIL_DEFAULT":
		return ErrorHandler{Action: ErrorActionDefault}
	case "ZBX_PREPROC_FAIL_DISCARD_VALUE":
		return ErrorHandler{Action: ErrorActionDiscard}
	case "ZBX_PREPROC_FAIL_SET_VALUE":
		return ErrorHandler{Action: ErrorActionSetValue, Params: paramStr}
	case "ZBX_PREPROC_FAIL_SET_ERROR":
		return ErrorHandler{Action: ErrorActionSetError, Params: paramStr}
	default:
		return ErrorHandler{Action: ErrorActionDefault}
	}
}

// ExpectedOutputFromTestCase gets expected output from test case.
func ExpectedOutputFromTestCase(tc TestCase) (string, error) {
	output := ""
	if tc.Out.Value != nil {
		output = fmt.Sprint(tc.Out.Value)
	}
	return output, nil
}
