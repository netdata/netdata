#ifndef NETDATA_EVAL_H
#define NETDATA_EVAL_H

/**
 * @file eval.h
 * @brief API to handle health expressions.
 */ 

#define EVAL_MAX_VARIABLE_NAME_LENGTH 300 ///< Maximum health variable name length

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

#define EVAL_VALUE_INVALID    0 ///< Expression is invalid.
#define EVAL_VALUE_NUMBER     1 ///< Expression is a number.
#define EVAL_VALUE_VARIABLE   2 ///< Expression is a variable.
#define EVAL_VALUE_EXPRESSION 3 ///< Expression is an expression.

// parsing and evaluation
#define EVAL_ERROR_OK                             0 ///< NO error while parsing.

// parsing errors
#define EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION    1 ///< Error while parsing. Missing close subexpression. 
#define EVAL_ERROR_UNKNOWN_OPERAND                2 ///< Error while parsing. Unknown operand.
#define EVAL_ERROR_MISSING_OPERAND                3 ///< Error while parsing. Missing operand.
#define EVAL_ERROR_MISSING_OPERATOR               4 ///< Error while parsing. Missing Operator.
#define EVAL_ERROR_REMAINING_GARBAGE              5 ///< Error while parsing. Garbage Remaining.
#define EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE      6 ///< Error while parsing. Missing else.

// evaluation errors
#define EVAL_ERROR_INVALID_VALUE                101 ///< Error during evaluation. Invalid value.
#define EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS   102 ///< Error during evaluation. Invalid number of operands.
#define EVAL_ERROR_VALUE_IS_NAN                 103 ///< Error during evaluation. Value is no number.
#define EVAL_ERROR_VALUE_IS_INFINITE            104 ///< Error during evaluation. Value is infinite.
#define EVAL_ERROR_UNKNOWN_VARIABLE             105 ///< Error during evaluation. Unknown variable

/** 
 * Parse `string` as an `EVAL_EXPRESSION`
 *
 * Parse the given string as an expression and return:
 * - a pointer to an expression if it parsed OK
 * - `NULL` in which case the pointer to error has the error code
 *
 * @param string Expression to parse.
 * @param failed_at Where the parsing failed.
 * @param error EVAL_ERROR_* if parsing failed.
 * @return the result.
 */
extern EVAL_EXPRESSION *expression_parse(const char *string, const char **failed_at, int *error);

/** 
 * Free an expression.
 *
 * Free all resources allocated for an expression `op`
 *
 * @param op Expression to free
 */
extern void expression_free(EVAL_EXPRESSION *op);

/** 
 * Convert an error code to a message.
 *
 * @param error EVAL_ERROR_*
 * @return a error message.
 */
extern const char *expression_strerror(int error);

/**
 * Evaluate an expression.
 * evaluate an expression and return
 * - 1 = OK, the result is in: expression->result
 * - 2 = FAILED, the error message is in: buffer_tostring(expression->error_msg)
 *
 * @param expression to evaluate
 * @return 1 on success, 2 on failure
 */
extern int expression_evaluate(EVAL_EXPRESSION *expression);

#endif //NETDATA_EVAL_H
