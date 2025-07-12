// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EVAL_H
#define NETDATA_EVAL_H 1

#include "../libnetdata.h"

#define EVAL_MAX_VARIABLE_NAME_LENGTH 300

struct eval_expression;
typedef struct eval_expression EVAL_EXPRESSION;
typedef bool (*eval_expression_variable_lookup_t)(STRING *variable, void *data, NETDATA_DOUBLE *result);

// parsing and evaluation error codes
typedef enum {
    EVAL_ERROR_OK = 0,

    // parsing errors
    EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION = 1,
    EVAL_ERROR_UNKNOWN_OPERAND = 2,
    EVAL_ERROR_MISSING_OPERAND = 3,
    EVAL_ERROR_MISSING_OPERATOR = 4,
    EVAL_ERROR_REMAINING_GARBAGE = 5,
    EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE = 6,

    // evaluation errors
    EVAL_ERROR_INVALID_VALUE = 101,
    EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS = 102,
    EVAL_ERROR_VALUE_IS_NAN = 103,
    EVAL_ERROR_VALUE_IS_INFINITE = 104,
    EVAL_ERROR_UNKNOWN_VARIABLE = 105,
    EVAL_ERROR_INVALID_OPERAND = 106,
    EVAL_ERROR_INVALID_OPERATOR = 107
} EVAL_ERROR;

// parse the given string as an expression and return:
//   a pointer to an expression if it parsed OK
//   NULL in which case the pointer to error has the error code
EVAL_EXPRESSION *expression_parse(const char *string, const char **failed_at, EVAL_ERROR *error);

// free all resources allocated for an expression
void expression_free(EVAL_EXPRESSION *expression);

// convert an error code to a message
const char *expression_strerror(EVAL_ERROR error);

// evaluate an expression and return
// 1 = OK, the result is in: expression->result
// 2 = FAILED, the error message is in: buffer_tostring(expression->error_msg)
int expression_evaluate(EVAL_EXPRESSION *expression);

const char *expression_source(EVAL_EXPRESSION *expression);
const char *expression_parsed_as(EVAL_EXPRESSION *expression);
const char *expression_error_msg(EVAL_EXPRESSION *expression);
NETDATA_DOUBLE expression_result(EVAL_EXPRESSION *expression);
void expression_set_variable_lookup_callback(EVAL_EXPRESSION *expression, eval_expression_variable_lookup_t cb, void *data);

void expression_hardcode_variable(EVAL_EXPRESSION *expression, STRING *variable, NETDATA_DOUBLE value);

// Dynamic function support
// Forward declaration for the eval_value structure
typedef struct eval_value EVAL_VALUE;

// Function callback type for dynamic functions
typedef NETDATA_DOUBLE (*eval_function_cb)(EVAL_EXPRESSION *exp, int param_count, EVAL_VALUE *params, EVAL_ERROR *error);

// Register a new function in the evaluation system
// name: The function name (case-insensitive)
// callback: The function implementation
// min_params: Minimum number of parameters required (must be >= 0)
// max_params: Maximum number of parameters allowed (-1 for unlimited)
// Returns: 1 on success, 0 on failure
bool eval_register_function(const char *name, eval_function_cb callback, int min_params, int max_params);

void eval_functions_init(void);

#endif //NETDATA_EVAL_H
