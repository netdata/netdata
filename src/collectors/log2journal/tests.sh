#!/usr/bin/env bash

# Improved log2journal test framework with .cmd and .fail support

set -e

# Parse command line arguments
VERBOSE=false
SPECIFIC_TEST=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --verbose)
            VERBOSE=true
            shift
            ;;
        --test)
            SPECIFIC_TEST="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [OPTIONS]"
            echo "Options:"
            echo "  --verbose     Show exact commands and full diff output"
            echo "  --test NAME   Run only the specified test"
            echo "  --help        Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  TESTED_LOG2JOURNAL_BIN   Path to log2journal binary (default: log2journal)"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

TEST_DIR="tests.d"
RESULTS_DIR="/tmp/log2journal_test_results"

# Use installed log2journal by default, allow override via environment variable
if [ -z "${TESTED_LOG2JOURNAL_BIN}"  -a ! -z "${LOG2JOURNAL}" ]; then
    export TESTED_LOG2JOURNAL_BIN="${LOG2JOURNAL}"
fi
if [ -z "$TESTED_LOG2JOURNAL_BIN" ]; then
    export TESTED_LOG2JOURNAL_BIN="log2journal"
fi

echo "log2journal cmd: ${TESTED_LOG2JOURNAL_BIN}"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Counters
TESTS_RUN=0
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_IGNORED=0

# Create results directory
mkdir -p "$RESULTS_DIR"

# Function to run a single test
run_test() {
    local test_name="$1"
    local yaml_file="$2"
    local input_file="$3"
    local expected_output="$4"
    local expected_config="$5"
    local cmd_file="$6"
    local fail_file="$7"
    
    TESTS_RUN=$((TESTS_RUN + 1))
    local test_passed=true
    local error_msg=""
    
    # Determine command to run
    local cmd=""
    if [ -f "$cmd_file" ]; then
        # Use custom command from .cmd file
        cmd=$(envsubst < "$cmd_file")
    elif [ -f "$yaml_file" ]; then
        # Standard YAML file test
        cmd="$TESTED_LOG2JOURNAL_BIN -f $yaml_file"
    else
        # Internal config test (extract config name from test_name)
        if [[ "$test_name" =~ ^(default|nginx-combined|nginx-json|logfmt)$ ]]; then
            cmd="$TESTED_LOG2JOURNAL_BIN -c $test_name"
        else
            echo -e "${YELLOW}IGNORED${NC} (no config or cmd file)"
            TESTS_IGNORED=$((TESTS_IGNORED + 1))
            return
        fi
    fi
    
    # Determine input source
    local input_args=""
    if [ -f "$input_file" ]; then
        input_args="< $input_file"
    else
        input_args="< /dev/null"
    fi
    
    if [ "$VERBOSE" = true ]; then
        echo "Running test: $test_name"
        echo "  Command: $cmd $input_args"
    else
        echo -n "Running test: $test_name ... "
    fi
    
    # Check if this is a failure test
    if [ -f "$fail_file" ]; then
        # Test should fail
        local actual_error="$RESULTS_DIR/${test_name}.err"
        if eval "$cmd $input_args" > /dev/null 2> "$actual_error"; then
            test_passed=false
            error_msg="Test was expected to fail but succeeded"
        else
            # Command failed as expected, check error message if fail file has content
            if [ -s "$fail_file" ]; then
                if ! grep -qF "$(cat "$fail_file")" "$actual_error"; then
                    test_passed=false
                    error_msg="Error message mismatch - see $actual_error vs $fail_file"
                fi
            fi
        fi
    else
        # Normal test - check output
        if [ -f "$expected_output" ]; then
            local actual_output="$RESULTS_DIR/${test_name}.out"
            
            if ! eval "$cmd $input_args" > "$actual_output" 2> "$RESULTS_DIR/${test_name}.err"; then
                test_passed=false
                error_msg="Command failed with non-zero exit code - see $RESULTS_DIR/${test_name}.err"
            else
                # Only check output if command succeeded
                # For help/error output tests, ignore version lines to avoid build-dependent failures
            if [[ "$test_name" =~ ^error- ]] && grep -q "^Netdata log2journal v" "$expected_output"; then
                # Version-agnostic comparison for error tests
                grep -v "^Netdata log2journal v" "$expected_output" > "$RESULTS_DIR/${test_name}.expected_no_version"
                grep -v "^Netdata log2journal v" "$actual_output" > "$RESULTS_DIR/${test_name}.actual_no_version"
                if ! diff -u "$RESULTS_DIR/${test_name}.expected_no_version" "$RESULTS_DIR/${test_name}.actual_no_version" > "$RESULTS_DIR/${test_name}.diff" 2>&1; then
                    test_passed=false
                    if [ "$VERBOSE" = true ]; then
                        error_msg="Output mismatch (version-agnostic):\n$(cat "$RESULTS_DIR/${test_name}.diff")"
                    else
                        error_msg="Output mismatch (version-agnostic) - see $RESULTS_DIR/${test_name}.diff"
                    fi
                fi
                
                # Also verify version format is correct
                if ! grep -q "^Netdata log2journal v[0-9]\+\.[0-9]\+\.[0-9]\+-[0-9]\+-g[a-f0-9]\+$" "$actual_output"; then
                    test_passed=false
                    error_msg="$error_msg\nVersion format is incorrect"
                fi
            else
                # Normal comparison for non-version-dependent tests
                if ! diff -u "$expected_output" "$actual_output" > "$RESULTS_DIR/${test_name}.diff" 2>&1; then
                    test_passed=false
                    if [ "$VERBOSE" = true ]; then
                        error_msg="Output mismatch:\n$(cat "$RESULTS_DIR/${test_name}.diff")"
                    else
                        error_msg="Output mismatch - see $RESULTS_DIR/${test_name}.diff"
                    fi
                fi
            fi
            fi
        fi
        
        # Check config output if expected
        if [ -f "$expected_config" ]; then
            local actual_config="$RESULTS_DIR/${test_name}-config.yaml"
            
            eval "$cmd --show-config $input_args" 2>/dev/null | sed '1,/^$/d' > "$actual_config" || true
            
            if ! diff -u "$expected_config" "$actual_config" > "$RESULTS_DIR/${test_name}-config.diff" 2>&1; then
                test_passed=false
                if [ "$VERBOSE" = true ]; then
                    error_msg="$error_msg\nConfig mismatch:\n$(cat "$RESULTS_DIR/${test_name}-config.diff")"
                else
                    error_msg="$error_msg\nConfig mismatch - see $RESULTS_DIR/${test_name}-config.diff"
                fi
            fi
        fi
    fi
    
    # Report result
    if [ "$test_passed" = true ]; then
        echo -e "${GREEN}PASSED${NC}"
        TESTS_PASSED=$((TESTS_PASSED + 1))
    else
        echo -e "${RED}FAILED${NC}"
        echo -e "$error_msg"
        TESTS_FAILED=$((TESTS_FAILED + 1))
    fi
}

# Main test loop
echo "Starting log2journal unit tests (v2 framework)..."
echo "================================"

# Find all unique test names by looking for any test files
if [ -n "$SPECIFIC_TEST" ]; then
    test_names="$SPECIFIC_TEST"
    echo "Running specific test: $SPECIFIC_TEST"
else
    test_names=$(find "$TEST_DIR" -name "*.yaml" -o -name "*.input" -o -name "*.output" -o -name "*.cmd" -o -name "*.fail" -o -name "*-final-config.yaml" | \
        sed -E 's/.*\/([^\/]+)\.(yaml|input|output|cmd|fail|-final-config\.yaml)$/\1/' | \
        sed 's/-final-config$//' | \
        sort -u)
fi

for test_name in $test_names; do
    # Check what files exist for this test
    yaml_file="$TEST_DIR/${test_name}.yaml"
    input_file="$TEST_DIR/${test_name}.input"
    output_file="$TEST_DIR/${test_name}.output"
    config_file="$TEST_DIR/${test_name}-final-config.yaml"
    cmd_file="$TEST_DIR/${test_name}.cmd"
    fail_file="$TEST_DIR/${test_name}.fail"
    
    # Check if test has any actual test files
    if [ ! -f "$output_file" ] && [ ! -f "$config_file" ] && [ ! -f "$fail_file" ]; then
        echo "Warning: Test $test_name has no expected output, config, or fail files, skipping"
        TESTS_IGNORED=$((TESTS_IGNORED + 1))
        continue
    fi
    
    run_test "$test_name" "$yaml_file" "$input_file" "$output_file" "$config_file" "$cmd_file" "$fail_file"
done

# Summary
echo "================================"
echo "Test Summary:"
echo "  Total tests run: $TESTS_RUN"
echo -e "  Passed: ${GREEN}$TESTS_PASSED${NC}"
echo -e "  Failed: ${RED}$TESTS_FAILED${NC}"
if [ $TESTS_IGNORED -gt 0 ]; then
    echo -e "  Ignored: ${YELLOW}$TESTS_IGNORED${NC}"
fi

if [ $TESTS_FAILED -gt 0 ]; then
    echo -e "\n${RED}TESTS FAILED${NC}"
    exit 1
else
    echo -e "\n${GREEN}ALL TESTS PASSED${NC}"
    exit 0
fi
