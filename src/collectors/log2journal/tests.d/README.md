# Log2journal Test Framework

This directory contains the comprehensive test suite for log2journal YAML parsing and functionality.

## Test Framework Overview

The test framework supports multiple test file formats for different testing scenarios:

### Standard Tests
1. **`{testname}.yaml`** - YAML configuration file (optional if using internal config)
2. **`{testname}.input`** - Input log lines to process (optional if testing with empty input)
3. **`{testname}.output`** - Expected output after processing

### CLI Tests (Improved Format)
1. **`{testname}.cmd`** - Command to execute with `${TESTED_LOG2JOURNAL_BIN}` variable
2. **`{testname}.yaml`** - Base configuration (optional)
3. **`{testname}.input`** - Input log lines
4. **`{testname}.output`** - Expected output

### Failure Tests
1. **`{testname}.yaml`** - Configuration that should fail
2. **`{testname}.input`** - Input log lines (optional)
3. **`{testname}.fail`** - Expected error message (or empty for any failure)

### Config Display Tests
1. **`{testname}.yaml`** - Configuration file
2. **`{testname}-final-config.yaml`** - Expected `--show-config` output

## Test Framework (Modern Format Only)

The test framework now uses clean, explicit file formats:

### CLI Format
```bash
# Clean command files:
# inject-append.cmd
${TESTED_LOG2JOURNAL_BIN} -f inject-append.yaml --inject CLINEWKEY1=value1 --inject CLINEWKEY2=value2
```

### Error Testing Format
```bash
# Explicit failure tests:
# error-test.fail
YAML PARSER: syntax error at line 5
```

## Running Tests

### Default (uses installed log2journal)
```bash
./tests.sh
```

### With custom binary
```bash
# Set the binary to test (e.g., local build)
export TESTED_LOG2JOURNAL_BIN="../../../build/log2journal"
./tests.sh
```

## Test Categories

### 1. Core Logic Tests (`logic-*.yaml` - 7 tests)
Tests fundamental log2journal functionality:
- **logic-rewrite-pipeline** - Rewrite pipeline with stop/continue behavior
- **logic-variable-substitution** - Variable replacement with ${VAR} syntax
- **logic-rename-chains** - Field renaming functionality
- **logic-filter-behavior** - Include/exclude filter patterns
- **logic-unmatched-handling** - Behavior for unmatched log lines
- **logic-pcre2-groups** - PCRE2 named capture groups
- **logic-key-validation** - Journal key naming validation

### 2. YAML Parsing Tests (`risk-*.yaml`, `yaml-*.yaml` - 13 tests)
Tests YAML parsing edge cases:
- **risk-yaml-constructs** - Complex YAML structures
- **risk-duplicate-behavior** - Duplicate key handling
- **risk-error-recovery** - Parser error recovery
- **risk-variable-edge-cases** - Variable substitution edge cases
- **yaml-multiline-complex** - Multiline strings, folded/literal scalars
- **yaml-edge-cases** - YAML type handling (bools, numbers, strings)
- **yaml-strings-comprehensive** - All YAML string quoting styles

### 3. Unicode and Encoding Tests (`unicode-*.yaml`, `encoding-*.yaml` - 8 tests)
Critical for log processing:
- **unicode-utf8** - UTF-8 multibyte characters, emojis
- **unicode-test** - Unicode in patterns and variable substitution
- **unicode-escape-sequences** - Unicode escape sequences (\uXXXX)
- **unicode-control-chars** - Control characters in logs
- **encoding-special-chars** - Control characters, special symbols
- **variable-substitution-unicode** - Unicode in ${VAR} substitutions

### 4. Error Handling Tests (`error-*.yaml` - 6 tests)
Tests that should fail gracefully:
- **error-invalid-syntax** - Invalid YAML syntax
- **error-missing-pattern** - Missing required fields
- **error-wrong-types** - Wrong data types
- **error-invalid-regex** - Invalid PCRE2 patterns
- **error-messages-validation** - Error message validation
- **error-recovery-comprehensive** - Comprehensive error scenarios

### 5. Advanced Feature Tests (`advanced-*.yaml` - 5 tests)
Complex functionality:
- **advanced-prefix** - Prefix application to all keys
- **advanced-filter** - Complex include/exclude patterns
- **advanced-unmatched** - Unmatched line handling with injection
- **advanced-pcre2-complex** - Complex PCRE2 patterns
- **advanced-edge-cases** - Combined edge cases

### 6. CLI Integration Tests (2 tests)
Test CLI/config interaction:
- **inject-append** - CLI appends to inject rules
- **filter-cli** - CLI filter behavior

### 7. Internal Config Tests
Using `-c` flag with built-in configs:
- **default** - Default configuration
- **nginx-combined** - Nginx combined log format
- **nginx-json** - Nginx JSON log format
- **logfmt** - Logfmt parsing

### 8. Boundary Tests (`boundary-*.yaml`, `edge-*.yaml` - 12 tests)
System limits and edge cases:
- **boundary-empty-config** - Minimal valid configuration
- **boundary-max-items** - Maximum array sizes (512 items)
- **boundary-key-length** - 64-character field limit testing
- **edge-empty-values** - Empty strings and values
- **edge-long-strings** - Very long input strings
- **edge-special-chars** - Special character handling

### 9. Real-World Tests (`real-world-*.yaml` - 5 tests)
Practical log format examples:
- **real-world-apache-logs** - Apache access log parsing
- **real-world-nginx-error** - Nginx error log format
- **real-world-docker-logs** - Docker container log format
- **real-world-syslog** - Traditional syslog parsing
- **real-world-multiline-stack** - Java stack trace handling

### 10. Full Integration Tests
- **full** - Complete configuration with all features

## Creating New Tests

### Standard Test
```bash
# Create input file
echo "your test log line" > tests.d/mytest.input

# Create YAML config
cat > tests.d/mytest.yaml << EOF
pattern: 'your pattern'
# other config...
EOF

# Generate expected output
cat tests.d/mytest.input | $TESTED_LOG2JOURNAL_BIN -f tests.d/mytest.yaml > tests.d/mytest.output
```

### CLI Test
```bash
# Create command file
echo '${TESTED_LOG2JOURNAL_BIN} -f mytest.yaml --inject KEY=value' > tests.d/mytest.cmd

# Create config and input
echo 'pattern: "(?P<MESSAGE>.*)"' > tests.d/mytest.yaml
echo 'test input' > tests.d/mytest.input

# Generate expected output (using your build)
cat tests.d/mytest.input | $TESTED_LOG2JOURNAL_BIN -f tests.d/mytest.yaml --inject KEY=value > tests.d/mytest.output
```

### Failure Test
```bash
# Create config that should fail
echo 'invalid: yaml: syntax:' > tests.d/fail-test.yaml

# Create expected error message
echo 'YAML PARSER: syntax error' > tests.d/fail-test.fail

# Optionally create input
echo 'test input' > tests.d/fail-test.input
```

### Show-Config Test
```bash
# Test with --show-config to verify CLI/YAML merging
echo "test" | $TESTED_LOG2JOURNAL_BIN -f tests.d/config.yaml --some-arg value --show-config | sed '1,/^$/d' > tests.d/testname-final-config.yaml
```

## Test Implementation Details

### Pattern Types
- **PCRE2 pattern**: Custom regex with named groups `(?P<name>...)`
- **`json`**: Parse JSON formatted logs
- **`logfmt`**: Parse logfmt formatted logs

### Variable Substitution
- `${VARIABLE}`: Replaced with variable value
- `${undefined}`: Replaced with empty string
- Variables can reference:
  - Captured groups from pattern
  - Other injected keys
  - Renamed keys (after renaming)

### Processing Pipeline
1. **EXTRACT** - Pattern matching extracts fields
2. **PREFIX** - Apply prefix to all keys
3. **RENAME** - Rename keys (currently broken)
4. **INJECT** - Add constant fields
5. **REWRITE** - Modify field values (currently broken)
6. **FILTER** - Include/exclude fields
7. **OUTPUT** - Generate Journal Export Format

### Key Behaviors

#### Duplicate Keys
- In `inject`: All values are added (allows duplicates)
- In `rewrite`: All rules processed in order (pipeline)
- In `rename`: All renames attempted

#### CLI Precedence
- **PREFIX**: CLI replaces config
- **INJECT**: CLI prepends to config
- **FILTER**: CLI replaces config
- **REWRITE/RENAME**: CLI arguments ignored (features broken)

#### Journal Field Rules
- Field names: max 64 characters
- Only A-Z, 0-9, underscore allowed
- First character cannot be digit
- All keys converted to uppercase
- Non-alphanumeric → underscore

### Known Issues
1. **Rewrite feature is broken** - Values never change
2. **Rename feature is broken** - Simple rename works, chaining doesn't
3. **Unmatched lines** - Currently accepts all input (pattern not enforced)

## Test Results

Results stored in `/tmp/log2journal_test_results/`:
- `{testname}.out` - Actual output
- `{testname}.err` - Error output
- `{testname}.diff` - Difference from expected
- `{testname}-config.yaml` - Actual config from --show-config

## Current Test Coverage

- **Total tests**: 81
- **Categories covered**: All major features  
- **Success rate**: 100%

### Coverage by Feature
- ✅ Pattern matching (PCRE2, JSON, logfmt)
- ✅ Variable substitution
- ✅ Prefix functionality
- ✅ Inject functionality
- ✅ Filter (include/exclude)
- ✅ Unicode/UTF-8 handling
- ✅ CLI argument precedence
- ✅ YAML parsing edge cases
- ⚠️  Rewrite (broken, tests document this)
- ⚠️  Rename (partially working)
- ❌ Filename tracking (pipe mode only)

## Troubleshooting Failed Tests

The test framework includes built-in debugging capabilities:

### Verbose Mode
Show exact commands and full diff output:
```bash
./tests.sh --verbose --test {test-name}
```

### Run Specific Test
Test only one specific case:
```bash
./tests.sh --test {test-name}
```

### Debug Methodology
1. **Identify the failing test** from test summary
2. **Run with verbose output** to see exact command and diff
3. **Check test files** to understand expected behavior:
   - `.yaml` - Configuration file
   - `.input` - Input log lines  
   - `.output` - Expected output
   - `.cmd` - Custom command (overrides default)
   - `.fail` - Expected error message (for failure tests)
4. **Run command manually** to verify behavior:
   ```bash
   export TESTED_LOG2JOURNAL_BIN="/path/to/your/log2journal"
   cat tests.d/{test}.input | $TESTED_LOG2JOURNAL_BIN -f tests.d/{test}.yaml
   ```
5. **Update expected output** if behavior is correct:
   ```bash
   cat tests.d/{test}.input | $TESTED_LOG2JOURNAL_BIN -f tests.d/{test}.yaml > tests.d/{test}.output
   ```

### Common Issues
- **Path differences**: Error messages may include full binary paths
- **Missing binary**: Set `TESTED_LOG2JOURNAL_BIN` to correct path
- **Output changes**: Use verbose mode to see actual vs expected output
- **CLI argument issues**: Check `.cmd` file for correct syntax
- **Version differences**: Error test outputs automatically ignore version lines to prevent build-dependent failures

### Version-Agnostic Testing
For tests that include version information (like `error-*` tests):
- The framework automatically ignores version lines during comparison
- Expected output files can contain placeholder versions (e.g., `v0.0.0-0-g00000000`)
- Version format is still validated to ensure it matches the expected pattern
- This prevents tests from failing when the build version changes

## Framework Environment Variables

- **`TESTED_LOG2JOURNAL_BIN`** - Path to log2journal binary for testing (default: `log2journal` from PATH)
- Set before running `tests.sh` to test different builds

The test framework provides comprehensive coverage of log2journal functionality with clean, maintainable test definitions.