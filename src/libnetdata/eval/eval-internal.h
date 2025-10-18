// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EVAL_INTERNAL_H
#define NETDATA_EVAL_INTERNAL_H

#include "eval.h"

typedef enum __attribute__((packed)) {
    EVAL_VALUE_INVALID = 0,
    EVAL_VALUE_NUMBER,
    EVAL_VALUE_VARIABLE,
    EVAL_VALUE_EXPRESSION
} EVAL_VALUE_TYPE;

// these are used for EVAL_NODE.operator
// they are used as internal IDs to identify an operator
typedef enum __attribute__((packed)) {
    EVAL_OPERATOR_NOP = 0,
    EVAL_OPERATOR_EXPRESSION_OPEN,
    EVAL_OPERATOR_EXPRESSION_CLOSE,
    EVAL_OPERATOR_NOT,
    EVAL_OPERATOR_PLUS,
    EVAL_OPERATOR_MINUS,
    EVAL_OPERATOR_AND,
    EVAL_OPERATOR_OR,
    EVAL_OPERATOR_GREATER_THAN_OR_EQUAL,
    EVAL_OPERATOR_LESS_THAN_OR_EQUAL,
    EVAL_OPERATOR_NOT_EQUAL,
    EVAL_OPERATOR_EQUAL,
    EVAL_OPERATOR_LESS,
    EVAL_OPERATOR_GREATER,
    EVAL_OPERATOR_MULTIPLY,
    EVAL_OPERATOR_DIVIDE,
    EVAL_OPERATOR_MODULO,
    EVAL_OPERATOR_SIGN_PLUS,
    EVAL_OPERATOR_SIGN_MINUS,
    EVAL_OPERATOR_ABS,                          // for the legacy parser - it is not used by re2c/lemon
    EVAL_OPERATOR_IF_THEN_ELSE,
    EVAL_OPERATOR_ASSIGNMENT,
    EVAL_OPERATOR_SEMICOLON,
    EVAL_OPERATOR_FUNCTION,                     // Generic function operator

    // terminator
    EVAL_OPERATOR_CUSTOM_FUNCTION_START,        // Start of custom function operators
    EVAL_OPERATOR_CUSTOM_FUNCTION_END = 255,    // End of custom function operators
} EVAL_OPERATOR;

// ----------------------------------------------------------------------------
// data structures for storing the parsed expression in memory

typedef struct eval_variable {
    STRING *name;
    struct eval_variable *next;
} EVAL_VARIABLE;

typedef struct eval_value {
    EVAL_VALUE_TYPE type;

    union {
        NETDATA_DOUBLE number;
        EVAL_VARIABLE *variable;
        struct eval_node *expression;
    };
} EVAL_VALUE;

typedef struct eval_node {
    int id;
    EVAL_OPERATOR operator;
    int precedence;

    int count;
    EVAL_VALUE ops[];
} EVAL_NODE;

// Forward declaration
struct eval_expression;

// Dynamic function callback type - accepts variable number of parameters
typedef NETDATA_DOUBLE (*eval_function_cb)(struct eval_expression *exp, int param_count, EVAL_VALUE *params, EVAL_ERROR *error);

// Definition of operators structure
struct operator {
    const char *print_as;
    char precedence;
    char parameters;
    char isfunction;
    NETDATA_DOUBLE (*eval)(struct eval_expression *exp, EVAL_NODE *op, EVAL_ERROR *error);
};

// External declaration of operators array (defined in eval-execute.c)
extern struct operator operators[256];

// Structure to store local variables in the expression
typedef struct eval_local_variable {
    STRING *name;
    NETDATA_DOUBLE value;
    struct eval_local_variable *next;
} EVAL_LOCAL_VARIABLE;

// Dynamic function registry entry structure
typedef struct eval_dynamic_function {
    STRING *name;                 // Function name (case-insensitive)
    eval_function_cb callback;    // Function implementation
    int min_params;               // Minimum number of parameters
    int max_params;               // Maximum number of parameters (-1 for unlimited)
    EVAL_OPERATOR operator;       // Unique operator ID for this function
    struct eval_dynamic_function *next;
} EVAL_DYNAMIC_FUNCTION;

struct eval_expression {
    STRING *source;
    STRING *parsed_as;

    NETDATA_DOUBLE result;

    EVAL_ERROR error;
    BUFFER *error_msg;

    EVAL_NODE *nodes;

    void *variable_lookup_cb_data;
    eval_expression_variable_lookup_t variable_lookup_cb;
    
    // Local variables defined within the expression
    EVAL_LOCAL_VARIABLE *local_variables;
};

// Function identifiers for parsing
typedef struct eval_function {
    const char *name;      // Function name (lower case)
    EVAL_OPERATOR op;      // Operator ID
    int precedence;        // Operator precedence
} EVAL_FUNCTION;

// Dynamic function registry functions
extern EVAL_DYNAMIC_FUNCTION *eval_function_registry;
extern EVAL_OPERATOR next_function_op;

// Register a new function in the registry
bool eval_register_function(const char *name, eval_function_cb callback, int min_params, int max_params);

// Lookup a function by name (case insensitive)
EVAL_DYNAMIC_FUNCTION *eval_function_lookup(const char *name);

// Function to evaluate a dynamic function
NETDATA_DOUBLE eval_execute_function(struct eval_expression *exp, EVAL_NODE *op, EVAL_ERROR *error);

bool has_the_right_number_of_operands(EVAL_NODE *op);

// Function declarations for shared functions

// From eval-utils.c
EVAL_NODE *eval_node_alloc(int count);
void eval_node_set_value_to_node(EVAL_NODE *op, int pos, EVAL_NODE *value);
void eval_node_set_value_to_constant(EVAL_NODE *op, int pos, NETDATA_DOUBLE value);
void eval_node_set_value_to_variable(EVAL_NODE *op, int pos, const char *variable);
void eval_variable_free(EVAL_VARIABLE *v);
void eval_value_free(EVAL_VALUE *v);
void eval_node_free(EVAL_NODE *op);
void print_parsed_as_variable(BUFFER *out, EVAL_VARIABLE *v, EVAL_ERROR *error);
void print_parsed_as_constant(BUFFER *out, NETDATA_DOUBLE n);
void print_parsed_as_value(BUFFER *out, EVAL_VALUE *v, EVAL_ERROR *error);
void print_parsed_as_node(BUFFER *out, EVAL_NODE *op, EVAL_ERROR *error);

// From eval-execute.c
NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error);
NETDATA_DOUBLE eval_value(EVAL_EXPRESSION *exp, EVAL_VALUE *v, EVAL_ERROR *error);
char eval_precedence(EVAL_OPERATOR operator);

// Functions for variable handling
NETDATA_DOUBLE get_local_variable_value(EVAL_EXPRESSION *exp, STRING *var_name, EVAL_ERROR *error);
void set_local_variable_value(EVAL_EXPRESSION *exp, STRING *var_name, NETDATA_DOUBLE value);

// Functions for other parsers
EVAL_NODE *parse_expression_with_re2c_lemon(const char *string, const char **failed_at, EVAL_ERROR *error);

// Parser selection - comment/uncomment to switch between implementations
#define USE_RE2C_LEMON_PARSER

#endif //NETDATA_EVAL_INTERNAL_H
