#ifndef NETDATA_EVAL_H
#define NETDATA_EVAL_H

typedef struct variable {
    char *name;
    struct rrdvar *rrdvar;
    struct variable *next;
} VARIABLE;

#define EVAL_VALUE_INVALID 0
#define EVAL_VALUE_NUMBER 1
#define EVAL_VALUE_VARIABLE 2
#define EVAL_VALUE_EXPRESSION 3

// these are used for EVAL_OPERAND.operator
#define EVAL_OPERATOR_NOP                   '\0'
#define EVAL_OPERATOR_VALUE                 ':'
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
#define EVAL_OPERATOR_SIGN_PLUS             'P'
#define EVAL_OPERATOR_SIGN_MINUS            'M'

#define EVAL_ERROR_OK 0
#define EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION 1
#define EVAL_ERROR_UNKNOWN_OPERAND 2
#define EVAL_ERROR_MISSING_OPERAND 3
#define EVAL_ERROR_MISSING_OPERATOR 4

typedef struct eval_value {
    int type;

    union {
        calculated_number number;
        VARIABLE *variable;
        struct eval_operand *expression;
    };
} EVAL_VALUE;

typedef struct eval_operand {
    int id;
    char operator;
    int precedence;

    int count;
    EVAL_VALUE ops[];
} EVAL_OPERAND;

extern EVAL_OPERAND *parse_expression(const char *string, const char **failed_at, int *error);
extern void free_expression(EVAL_OPERAND *op);

#endif //NETDATA_EVAL_H
