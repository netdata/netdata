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
    unsigned char operator;
    int precedence;

    int count;
    EVAL_VALUE ops[];
} EVAL_NODE;

// Definition of operators structure
struct operator {
    const char *print_as;
    char precedence;
    char parameters;
    char isfunction;
    NETDATA_DOUBLE (*eval)(struct eval_expression *exp, EVAL_NODE *op, int *error);
};

// External declaration of operators array (defined in eval-execute.c)
extern struct operator operators[256];

struct eval_expression {
    STRING *source;
    STRING *parsed_as;

    NETDATA_DOUBLE result;

    int error;
    BUFFER *error_msg;

    EVAL_NODE *nodes;

    void *variable_lookup_cb_data;
    eval_expression_variable_lookup_t variable_lookup_cb;
};

// these are used for EVAL_NODE.operator
// they are used as internal IDs to identify an operator
// THEY ARE NOT USED FOR PARSING OPERATORS LIKE THAT
#define EVAL_OPERATOR_NOP                   '\0'
#define EVAL_OPERATOR_EXPRESSION_OPEN       '('
#define EVAL_OPERATOR_EXPRESSION_CLOSE      ')'
#define EVAL_OPERATOR_NOT                   '!'
#define EVAL_OPERATOR_PLUS                  '+'
#define EVAL_OPERATOR_MINUS                 '-'
#define EVAL_OPERATOR_AND                   '&'
#define EVAL_OPERATOR_OR                    '|'
#define EVAL_OPERATOR_GREATER_THAN_OR_EQUAL 'G'
#define EVAL_OPERATOR_LESS_THAN_OR_EQUAL    'L'
#define EVAL_OPERATOR_NOT_EQUAL             '~'
#define EVAL_OPERATOR_EQUAL                 '='
#define EVAL_OPERATOR_LESS                  '<'
#define EVAL_OPERATOR_GREATER               '>'
#define EVAL_OPERATOR_MULTIPLY              '*'
#define EVAL_OPERATOR_DIVIDE                '/'
#define EVAL_OPERATOR_MODULO                '%'
#define EVAL_OPERATOR_SIGN_PLUS             'P'
#define EVAL_OPERATOR_SIGN_MINUS            'M'
#define EVAL_OPERATOR_ABS                   'A'
#define EVAL_OPERATOR_IF_THEN_ELSE          '?'

// Function identifiers for parsing
typedef struct eval_function {
    const char *name;      // Function name (lower case)
    unsigned char op;      // Operator ID
    int precedence;        // Operator precedence
} EVAL_FUNCTION;

// Function declarations for shared functions

// From eval-utils.c
extern EVAL_NODE *eval_node_alloc(int count);
extern void eval_node_set_value_to_node(EVAL_NODE *op, int pos, EVAL_NODE *value);
extern void eval_node_set_value_to_constant(EVAL_NODE *op, int pos, NETDATA_DOUBLE value);
extern void eval_node_set_value_to_variable(EVAL_NODE *op, int pos, const char *variable);
extern void eval_variable_free(EVAL_VARIABLE *v);
extern void eval_value_free(EVAL_VALUE *v);
extern void eval_node_free(EVAL_NODE *op);
extern void print_parsed_as_variable(BUFFER *out, EVAL_VARIABLE *v, int *error);
extern void print_parsed_as_constant(BUFFER *out, NETDATA_DOUBLE n);
extern void print_parsed_as_value(BUFFER *out, EVAL_VALUE *v, int *error);
extern void print_parsed_as_node(BUFFER *out, EVAL_NODE *op, int *error);

// From eval-execute.c
extern NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error);
extern int eval_precedence(unsigned char operator);

// Functions for bison/flex integration
extern EVAL_NODE *parse_expression_with_bison(const char *string, const char **failed_at, int *error);

#endif //NETDATA_EVAL_INTERNAL_H