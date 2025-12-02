// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// ----------------------------------------------------------------------------
// evaluation of expressions

ALWAYS_INLINE
static NETDATA_DOUBLE eval_variable(EVAL_EXPRESSION *exp, EVAL_VARIABLE *v, int *error) {
    NETDATA_DOUBLE n;

    // Check if variable is NULL to avoid crashes
    if (!v || !v->name) {
        *error = EVAL_ERROR_UNKNOWN_VARIABLE;
        buffer_strcat(exp->error_msg, "[ undefined variable ] ");
        return NAN;
    }

    if(exp->variable_lookup_cb && exp->variable_lookup_cb(v->name, exp->variable_lookup_cb_data, &n)) {
        buffer_sprintf(exp->error_msg, "[ ${%s} = ", string2str(v->name));
        print_parsed_as_constant(exp->error_msg, n);
        buffer_strcat(exp->error_msg, " ] ");
        return n;
    }

    *error = EVAL_ERROR_UNKNOWN_VARIABLE;
    buffer_sprintf(exp->error_msg, "[ undefined variable '%s' ] ", string2str(v->name));
    return NAN;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_value(EVAL_EXPRESSION *exp, EVAL_VALUE *v, int *error) {
    NETDATA_DOUBLE n;

    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            n = eval_node(exp, v->expression, error);
            break;

        case EVAL_VALUE_NUMBER:
            n = v->number;
            break;

        case EVAL_VALUE_VARIABLE:
            n = eval_variable(exp, v->variable, error);
            break;

        default:
            *error = EVAL_ERROR_INVALID_VALUE;
            n = 0;
            break;
    }

    return n;
}

ALWAYS_INLINE
static int is_true(NETDATA_DOUBLE n) {
    // Handle special cases safely
    if(isnan(n)) return 0;    // NaN is considered false
    if(isinf(n)) {
        // Infinity is considered true (positive or negative)
        return 1;
    }
    if(n == 0) return 0;      // Zero is considered false
    return 1;                 // Any other value is true
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_and(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return is_true(eval_value(exp, &op->ops[0], error)) && is_true(eval_value(exp, &op->ops[1], error));
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_or(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return is_true(eval_value(exp, &op->ops[0], error)) || is_true(eval_value(exp, &op->ops[1], error));
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_greater_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isgreaterequal(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_less_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return islessequal(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) && isnan(n2)) return 1;
    if(isinf(n1) && isinf(n2)) return 1;
    if(isnan(n1) || isnan(n2)) return 0;
    if(isinf(n1) || isinf(n2)) return 0;
    return considered_equal_ndd(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_not_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return !eval_equal(exp, op, error);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_less(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isless(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_greater(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isgreater(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 + n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 - n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_multiply(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 * n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_divide(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    if(isinf(n1) || isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return INFINITY;
    }
    
    if(n2 == 0) {
        // In Netdata, we treat all division by zero as INFINITE error
        // This ensures compatibility with existing code
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return n1 >= 0 ? INFINITY : -INFINITY;
    }
    
    return n1 / n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_modulo(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;

    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;

    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }

    if(isinf(n1) || isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return INFINITY;
    }

    if(n2 == 0) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return NAN;  // Modulo by zero is undefined
    }

    return fmod(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_nop(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return eval_value(exp, &op->ops[0], error);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_not(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return !is_true(eval_value(exp, &op->ops[0], error));
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_sign_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return eval_value(exp, &op->ops[0], error);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_sign_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(isnan(n1)) return NAN;
    if(isinf(n1)) return INFINITY;
    return -n1;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_abs(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(isnan(n1)) return NAN;
    if(isinf(n1)) return INFINITY;
    return ABS(n1);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_if_then_else(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    if(is_true(eval_value(exp, &op->ops[0], error)))
        return eval_value(exp, &op->ops[1], error);
    else
        return eval_value(exp, &op->ops[2], error);
}

// Define operators table - use the struct definition from eval-internal.h
struct operator operators[256] = {
    // this is a random access array
    // we always access it with a known EVAL_OPERATOR_X

    [EVAL_OPERATOR_AND]                   = { "&&", 2, 2, 0, eval_and },
    [EVAL_OPERATOR_OR]                    = { "||", 2, 2, 0, eval_or },
    [EVAL_OPERATOR_GREATER_THAN_OR_EQUAL] = { ">=", 3, 2, 0, eval_greater_than_or_equal },
    [EVAL_OPERATOR_LESS_THAN_OR_EQUAL]    = { "<=", 3, 2, 0, eval_less_than_or_equal },
    [EVAL_OPERATOR_NOT_EQUAL]             = { "!=", 3, 2, 0, eval_not_equal },
    [EVAL_OPERATOR_EQUAL]                 = { "==", 3, 2, 0, eval_equal },
    [EVAL_OPERATOR_LESS]                  = { "<",  3, 2, 0, eval_less },
    [EVAL_OPERATOR_GREATER]               = { ">",  3, 2, 0, eval_greater },
    [EVAL_OPERATOR_PLUS]                  = { "+",  4, 2, 0, eval_plus },
    [EVAL_OPERATOR_MINUS]                 = { "-",  4, 2, 0, eval_minus },
    [EVAL_OPERATOR_MULTIPLY]              = { "*",  5, 2, 0, eval_multiply },
    [EVAL_OPERATOR_DIVIDE]                = { "/",  5, 2, 0, eval_divide },
    [EVAL_OPERATOR_MODULO]                = { "%",  5, 2, 0, eval_modulo },
    [EVAL_OPERATOR_NOT]                   = { "!",  6, 1, 0, eval_not },
    [EVAL_OPERATOR_SIGN_PLUS]             = { "+",  6, 1, 0, eval_sign_plus },
    [EVAL_OPERATOR_SIGN_MINUS]            = { "-",  6, 1, 0, eval_sign_minus },
    [EVAL_OPERATOR_ABS]                   = { "abs(",6,1, 1, eval_abs },
    [EVAL_OPERATOR_IF_THEN_ELSE]          = { "?",  7, 3, 0, eval_if_then_else },
    [EVAL_OPERATOR_NOP]                   = { NULL, 8, 1, 0, eval_nop },
    [EVAL_OPERATOR_EXPRESSION_OPEN]       = { NULL, 8, 1, 0, eval_nop },

    // this should exist in our evaluation list
    [EVAL_OPERATOR_EXPRESSION_CLOSE]      = { NULL, 99, 1, 0, eval_nop }
};

// Helper function to get precedence
ALWAYS_INLINE
int eval_precedence(unsigned char operator) {
    return operators[(unsigned char)(operator)].precedence;
}

ALWAYS_INLINE
NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    if(unlikely(!op)) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return 0;
    }

    if(unlikely(op->count != operators[op->operator].parameters)) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return 0;
    }

    NETDATA_DOUBLE n = operators[op->operator].eval(exp, op, error);

    return n;
}

// ----------------------------------------------------------------------------
// public API for evaluation

ALWAYS_INLINE
int expression_evaluate(EVAL_EXPRESSION *expression) {
    expression->error = EVAL_ERROR_OK;

    buffer_reset(expression->error_msg);
    expression->result = eval_node(expression, expression->nodes, &expression->error);

    if(unlikely(isnan(expression->result))) {
        if(expression->error == EVAL_ERROR_OK)
            expression->error = EVAL_ERROR_VALUE_IS_NAN;
    }
    else if(unlikely(isinf(expression->result))) {
        if(expression->error == EVAL_ERROR_OK)
            expression->error = EVAL_ERROR_VALUE_IS_INFINITE;
    }
    else if(unlikely(expression->error == EVAL_ERROR_UNKNOWN_VARIABLE)) {
        // although there is an unknown variable
        // the expression was evaluated successfully
        expression->error = EVAL_ERROR_OK;
    }

    if(expression->error != EVAL_ERROR_OK) {
        expression->result = NAN;

        if(buffer_strlen(expression->error_msg))
            buffer_strcat(expression->error_msg, "; ");

        buffer_sprintf(expression->error_msg, "failed to evaluate expression with error %d (%s)", expression->error, expression_strerror(expression->error));
        return 0;
    }

    return 1;
}

void expression_free(EVAL_EXPRESSION *expression) {
    if(!expression) return;

    if(expression->nodes) eval_node_free(expression->nodes);
    string_freez((void *)expression->source);
    string_freez((void *)expression->parsed_as);
    buffer_free(expression->error_msg);
    freez(expression);
}
