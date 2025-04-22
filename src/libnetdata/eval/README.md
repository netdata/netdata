# Netdata Expression Evaluator

This directory contains Netdata's Expression Evaluator, a component for evaluating mathematical and logical expressions in Netdata's health monitoring, alerts, and data processing pipelines.

## Overview

The expression evaluator is a parser and interpreter for mathematical, logical, and comparison expressions. It supports:

- Arithmetic operations (`+`, `-`, `*`, `/`, `%`)
- Logical operations (`AND`/`&&`, `OR`/`||`, `NOT`/`!`)
- Comparison operations (`==`, `!=`, `>`, `>=`, `<`, `<=`)
- Ternary conditional operator (`? :`)
- Function calls (e.g., `abs()`)
- Variables (e.g., `$var1`)

Expressions are used in Netdata's alert definitions, and other areas that require dynamic computation.

## Implementation

The expression evaluator has two parser implementations:

1. **Original Recursive Descent Parser** - A handwritten parser in `eval-parser-legacy.c`
2. **re2c/Lemon Parser** - A more efficient parser using re2c for lexical analysis and Lemon for grammar parsing in the `re2c_lemon/` subdirectory

The implementation can be switched between these two parsers using the `USE_RE2C_LEMON_PARSER` define in `eval-internal.h`.

## Key Components

- **eval.h** - Public API for the expression evaluator
- **eval-internal.h** - Internal structures and parser selection switch
- **eval-parser.c** - Original recursive descent parser implementation
- **eval-execute.c** - Expression evaluation engine
- **eval-utils.c** - Helper functions for working with expression nodes
- **eval-unittest.c** - Comprehensive test suite for the evaluator
- **re2c_lemon/** - Subdirectory containing the re2c/Lemon-based parser implementation

## Expression Syntax

The evaluator supports a C-like syntax:

```
# Arithmetic
42 + 24
5 * (3 + 2)

# Comparisons 
$temp > 80
$load >= $threshold

# Logical operations
$cpu_util > 90 && $mem_usage > 80
$disk_full || $inode_usage > 95

# Ternary operator
$status == $WARNING ? 90 : 75

# Functions
abs($value)
```

Variables are prefixed with `$` and can be either simple names (`$var`) or use braces for complex names (`${variable name with spaces}`).

## Special Features

- Case-insensitive handling of logical operators: `AND`/`and`/`&&` are equivalent
- Support for special numeric literals: `nan` and `inf` (any capitalization)
- Short-circuit evaluation of logical operators
- NaN and Infinity handling in calculations

## Usage

To use the expression evaluator in Netdata code:

```c
#include "libnetdata/eval/eval.h"

// Parse an expression
const char *expr = "$value > 100 && $status != 0";
const char *failed_at = NULL;
int error = 0;
EVAL_EXPRESSION *exp = expression_parse(expr, &failed_at, &error);

if (!exp) {
    // Handle parsing error
    printf("Error parsing expression at: %s\n", failed_at);
    printf("Error code: %d (%s)\n", error, expression_strerror(error));
    return;
}

// Set up variable lookup callback
expression_set_variable_lookup_callback(exp, my_variable_lookup_function, my_data);

// Evaluate the expression
if (expression_evaluate(exp)) {
    // Get the result
    NETDATA_DOUBLE result = expression_result(exp);
    printf("Result: %f\n", result);
} else {
    // Handle evaluation error
    printf("Evaluation error: %s\n", expression_error_msg(exp));
}

// Free the expression
expression_free(exp);
```

## Testing

The evaluator includes a comprehensive test suite in `eval-unittest.c`. Run it using:

```
netdata -W evaltest
```

All these tests run also at CI.
