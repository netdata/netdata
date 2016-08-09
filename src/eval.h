#ifndef NETDATA_EVAL_H
#define NETDATA_EVAL_H

typedef struct variable {
    char *name;
    struct rrdvar *rrdvar;
    struct variable *next;
} VARIABLE;

#define EVAL_OPERAND_INVALID 0
#define EVAL_OPERAND_NUMBER 1
#define EVAL_OPERAND_VARIABLE 2
#define EVAL_OPERAND_EXPRESSION 3

// these are used for EVAL_OPERAND.operator
#define EVAL_OPERATOR_NOP   '\0'
#define EVAL_OPERATOR_NOT   '!'
#define EVAL_OPERATOR_PLUS  '+'
#define EVAL_OPERATOR_MINUS '-'

typedef struct eval_value {
    int type;

    union {
        calculated_number number;
        VARIABLE *variable;
        struct eval_operand *expression;
    };
} EVAL_VALUE;

typedef struct eval_operand {
    char operator;

    int count;
    EVAL_VALUE ops[];
} EVAL_OPERAND;

#endif //NETDATA_EVAL_H
