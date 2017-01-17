#ifndef NETDATA_EVAL_H
#define NETDATA_EVAL_H

#define EVAL_MAX_VARIABLE_NAME_LENGTH 300

/** List of `struct rrdvar`*/
typedef struct eval_variable {
    char *name;                 ///< Uniqe name
    uint32_t hash;              ///< Hash of `name`
    struct rrdvar *rrdvar;      ///< The variable
    struct eval_variable *next; ///< Next item in the list
} EVAL_VARIABLE;

/**
 * The internal representation infix expressions used in alarms.
 *
 * @author Costa Tsaousis
 *
 * The expression is split a tree. Each node is performing a calculation.
 */
typedef struct eval_expression {
    const char *source;    ///< The source string
    const char *parsed_as; ///< The passed expression

    int *status;             ///< EVAL_VALUE_*
    calculated_number *this; ///< Number calculated from expression
    time_t *after;           ///< Next eval_expression of this
    time_t *before;          ///< Preceding eval_expression of this

    calculated_number result; ///< Result of the expression

    int error;         ///< EVAL_ERROR_*
    BUFFER *error_msg; ///< Reason why `error` occured.

    /// hidden EVAL_NODE *
    void *nodes;

    /// custom data to be used for looking up variables
    struct rrdcalc *rrdcalc;
} EVAL_EXPRESSION;

#define EVAL_VALUE_INVALID    0
#define EVAL_VALUE_NUMBER     1
#define EVAL_VALUE_VARIABLE   2
#define EVAL_VALUE_EXPRESSION 3

// parsing and evaluation
#define EVAL_ERROR_OK                             0

// parsing errors
#define EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION    1
#define EVAL_ERROR_UNKNOWN_OPERAND                2
#define EVAL_ERROR_MISSING_OPERAND                3
#define EVAL_ERROR_MISSING_OPERATOR               4
#define EVAL_ERROR_REMAINING_GARBAGE              5
#define EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE      6

// evaluation errors
#define EVAL_ERROR_INVALID_VALUE                101
#define EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS   102
#define EVAL_ERROR_VALUE_IS_NAN                 103
#define EVAL_ERROR_VALUE_IS_INFINITE            104
#define EVAL_ERROR_UNKNOWN_VARIABLE             105

// parse the given string as an expression and return:
//   a pointer to an expression if it parsed OK
//   NULL in which case the pointer to error has the error code
extern EVAL_EXPRESSION *expression_parse(const char *string, const char **failed_at, int *error);

// free all resources allocated for an expression
extern void expression_free(EVAL_EXPRESSION *op);

// convert an error code to a message
extern const char *expression_strerror(int error);

// evaluate an expression and return
// 1 = OK, the result is in: expression->result
// 2 = FAILED, the error message is in: buffer_tostring(expression->error_msg)
extern int expression_evaluate(EVAL_EXPRESSION *expression);

#endif //NETDATA_EVAL_H
