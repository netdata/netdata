// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// ----------------------------------------------------------------------------
// evaluation of expressions

ALWAYS_INLINE
static NETDATA_DOUBLE eval_variable(EVAL_EXPRESSION *exp, EVAL_VARIABLE *v, EVAL_ERROR *error) {
    // Check if variable is NULL to avoid crashes
    if (!v || !v->name) {
        *error = EVAL_ERROR_UNKNOWN_VARIABLE;
        buffer_strcat(exp->error_msg, "[ undefined variable ] ");
        return NAN;
    }

    // Use the improved local variable lookup function
    NETDATA_DOUBLE value = get_local_variable_value(exp, v->name, error);
    
    if (*error == EVAL_ERROR_OK) {
        buffer_sprintf(exp->error_msg, "[ ${%s} = ", string2str(v->name));
        print_parsed_as_constant(exp->error_msg, value);
        buffer_strcat(exp->error_msg, " ] ");
        return value;
    }

    buffer_sprintf(exp->error_msg, "[ undefined variable '%s' ] ", string2str(v->name));
    return NAN;
}

ALWAYS_INLINE
NETDATA_DOUBLE eval_value(EVAL_EXPRESSION *exp, EVAL_VALUE *v, EVAL_ERROR *error) {
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
            n = NAN;
            break;
    }

    return n;
}

ALWAYS_INLINE
static bool is_true(NETDATA_DOUBLE n) {
    if(isnan(n)) return false;                      // NaN is considered false
    if(isinf(n)) return true;                       // Infinity is considered true (positive or negative)
    if(considered_equal_ndd(n, 0.0)) return false;  // Zero is considered false
    return true;                                    // Any other value is true
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_and(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // Short-circuit: if first value is false, don't evaluate second
    if(!is_true(n1)) return 0.0;
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return is_true(n2) ? 1.0 : 0.0;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_or(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // Short-circuit: if first value is true, don't evaluate second
    if(is_true(n1)) return 1.0;
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return is_true(n2) ? 1.0 : 0.0;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_greater_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return isgreaterequal(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_less_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors

    return islessequal(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // According to IEEE 754, NaN is never equal to anything, including itself
    if(isnan(n1) || isnan(n2)) return 0.0;
    
    // For infinities, we need to check the sign
    if(isinf(n1) && isinf(n2)) {
        // +Infinity equals +Infinity, -Infinity equals -Infinity
        return (signbit(n1) == signbit(n2)) ? 1.0 : 0.0;
    }
    
    // If only one value is infinite, they can't be equal
    if(isinf(n1) || isinf(n2)) return 0.0;
    
    // Regular comparison for finite values
    return considered_equal_ndd(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_not_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    return !eval_equal(exp, op, error);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_less(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return isless(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_greater(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return isgreater(n1, n2);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors like UNKNOWN_VARIABLE
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // NaN propagation - any operation with NaN results in NaN
    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Special case: Infinity + (-Infinity) = NaN (indeterminate form)
    if(isinf(n1) && isinf(n2) && signbit(n1) != signbit(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Other infinity cases
    if(isinf(n1) || isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return INFINITY;
    }
    
    return n1 + n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors like UNKNOWN_VARIABLE
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // NaN propagation
    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Special case: Infinity - Infinity = NaN (indeterminate form)
    if(isinf(n1) && isinf(n2) && signbit(n1) == signbit(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Other infinity cases
    if(isinf(n1) || isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        // Need to determine the sign of infinity correctly
        if(isinf(n1) && isinf(n2)) {
            // Different signs: result will match sign of first operand
            return signbit(n1) ? -INFINITY : INFINITY;
        }
        else if(isinf(n1)) {
            // n1 is infinity, n2 is finite: result has same sign as n1
            return signbit(n1) ? -INFINITY : INFINITY;
        }
        else {
            // n1 is finite, n2 is infinity: result has opposite sign of n2
            return signbit(n2) ? INFINITY : -INFINITY;
        }
    }
    
    return n1 - n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_multiply(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors like UNKNOWN_VARIABLE
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // NaN propagation
    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Special case: 0 * Infinity = NaN (indeterminate form)
    if((n1 == 0 && isinf(n2)) || (isinf(n1) && n2 == 0)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Infinity * finite number (not zero) cases
    if(isinf(n1) || isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        
        // Determine sign of the result (product of the signs)
        bool neg_result = (signbit(n1) != signbit(n2));
        return neg_result ? -INFINITY : INFINITY;
    }
    
    return n1 * n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_divide(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    // NaN propagation
    if(isnan(n1) || isnan(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Indeterminate form: 0/0 = NaN
    if(n1 == 0 && n2 == 0) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Indeterminate form: Infinity/Infinity = NaN
    if(isinf(n1) && isinf(n2)) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return NAN;
    }
    
    // Division by zero (when numerator is not zero): +/-Infinity
    if(n2 == 0) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return signbit(n1) ? -INFINITY : INFINITY;
    }
    
    // Other infinity cases with finite non-zero divisor
    if(isinf(n1)) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        // Result sign depends on signs of both operands
        bool neg_result = (signbit(n1) != signbit(n2));
        return neg_result ? -INFINITY : INFINITY;
    }
    
    // Finite divided by infinity = 0 (but with correct sign)
    if(isinf(n2)) {
        return 0.0; // IEEE 754 says this is a signed zero, but we'll return regular 0
    }
    
    return n1 / n2;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_nop(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    // No special error handling needed - just pass through the value and any error
    return n1;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_not(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    return is_true(n1) ? 0.0 : 1.0;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_sign_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    return n1;
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_sign_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    return -n1;
}

// this is used by the legacy parser - it is not used by re2c/lemon parser
ALWAYS_INLINE
static NETDATA_DOUBLE eval_abs(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    return ABS(n1);
}

ALWAYS_INLINE
static NETDATA_DOUBLE eval_if_then_else(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    NETDATA_DOUBLE condition = eval_value(exp, &op->ops[0], error);
    if(*error != EVAL_ERROR_OK) return NAN;  // Propagate previous errors
    
    if(is_true(condition))
        return eval_value(exp, &op->ops[1], error);
    else
        return eval_value(exp, &op->ops[2], error);
}

// Function to evaluate variable assignment
ALWAYS_INLINE
static NETDATA_DOUBLE eval_assignment(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    // First operand should be a variable node
    EVAL_NODE *var_node = op->ops[0].expression;
    if (!var_node || var_node->count < 1 || var_node->ops[0].type != EVAL_VALUE_VARIABLE) {
        *error = EVAL_ERROR_INVALID_OPERAND;
        return NAN;
    }
    
    // Get the variable name
    EVAL_VARIABLE *variable = var_node->ops[0].variable;
    if (!variable || !variable->name) {
        *error = EVAL_ERROR_INVALID_OPERAND;
        return NAN;
    }
    
    STRING *var_name = variable->name;
    
    // Evaluate the expression to be assigned
    NETDATA_DOUBLE result = eval_value(exp, &op->ops[1], error);
    if (*error != EVAL_ERROR_OK) {
        return NAN;
    }
    
    // Use the improved set_local_variable_value function
    set_local_variable_value(exp, var_name, result);
    
    buffer_sprintf(exp->error_msg, "[ $%s = ", string2str(var_name));
    print_parsed_as_constant(exp->error_msg, result);
    buffer_strcat(exp->error_msg, " ] ");
    
    return result;
}

// Function to evaluate semicolon-separated expressions
ALWAYS_INLINE
static NETDATA_DOUBLE eval_semicolon(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    // First evaluate the left expression and store its result
    // The side effects (variable assignments) from this will be used in the right expression
    NETDATA_DOUBLE left_result = eval_value(exp, &op->ops[0], error);
    if (*error != EVAL_ERROR_OK) {
        return NAN;
    }
    (void)left_result;  // Ignore the result of the left expression
    
    // Now evaluate the right expression with any new variable values set by the left expression
    NETDATA_DOUBLE right_result = eval_value(exp, &op->ops[1], error);
    // No need to check error here - just pass it through
    
    // Return the result of the right expression (the last expression in the sequence)
    return right_result;
}

// Define operators table - use the struct definition from eval-internal.h
struct operator operators[EVAL_OPERATOR_CUSTOM_FUNCTION_END + 1] = {
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
    [EVAL_OPERATOR_NOT]                   = { "!",  6, 1, 0, eval_not },
    [EVAL_OPERATOR_SIGN_PLUS]             = { "+",  6, 1, 0, eval_sign_plus },
    [EVAL_OPERATOR_SIGN_MINUS]            = { "-",  6, 1, 0, eval_sign_minus },

    // this is only used by the legacy parser - not used by re2c/lemon parser
    [EVAL_OPERATOR_ABS]                   = { "abs(",6,1, 1, eval_abs },

    [EVAL_OPERATOR_IF_THEN_ELSE]          = { "?",  7, 3, 0, eval_if_then_else },

    // Lower precedence than arithmetic
    [EVAL_OPERATOR_ASSIGNMENT]            = { "=",  1, 2, 0, eval_assignment },

    // Lowest precedence
    [EVAL_OPERATOR_SEMICOLON]             = { ";",  0, 2, 0, eval_semicolon },

    // Dynamic functions
    [EVAL_OPERATOR_FUNCTION]              = { NULL, 6, 0, 1, eval_execute_function },

    [EVAL_OPERATOR_NOP]                   = { NULL, 9, 1, 0, eval_nop },
    [EVAL_OPERATOR_EXPRESSION_OPEN]       = { NULL, 9, 1, 0, eval_nop },

    // this should exist in our evaluation list
    [EVAL_OPERATOR_EXPRESSION_CLOSE]      = { NULL, 99, 1, 0, eval_nop }
};

// Helper function to get precedence
ALWAYS_INLINE
char eval_precedence(EVAL_OPERATOR operator) {
    return operators[operator].precedence;
}

bool has_the_right_number_of_operands(EVAL_NODE *op) {
    return op->operator >= EVAL_OPERATOR_CUSTOM_FUNCTION_START || operators[op->operator].parameters == op->count;
}

ALWAYS_INLINE
NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    if(unlikely(!op)) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NAN;
    }

    if(op->operator >= EVAL_OPERATOR_CUSTOM_FUNCTION_END) {
        *error = EVAL_ERROR_INVALID_OPERATOR;
        return NAN;
    }

    if(unlikely(!has_the_right_number_of_operands(op))) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return NAN;
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

    // Only set NaN error if no other error is set
    if(unlikely(isnan(expression->result))) {
        if(expression->error == EVAL_ERROR_OK)
            expression->error = EVAL_ERROR_VALUE_IS_NAN;
    }
    // Only set Infinity error if no other error is set
    else if(unlikely(isinf(expression->result))) {
        if(expression->error == EVAL_ERROR_OK)
            expression->error = EVAL_ERROR_VALUE_IS_INFINITE;
    }
    
    // Keep the UNKNOWN_VARIABLE error - don't clear it!
    // This was causing UNKNOWN_VARIABLE to be cleared and resulting in passing 
    // when tests expected this specific error

    if(expression->error != EVAL_ERROR_OK) {
        expression->result = NAN;

        if(buffer_strlen(expression->error_msg))
            buffer_strcat(expression->error_msg, "; ");

        buffer_sprintf(expression->error_msg, "failed to evaluate expression with error %u (%s)", expression->error, expression_strerror(expression->error));
        return 0;
    }

    return 1;
}

void expression_free(EVAL_EXPRESSION *expression) {
    if(!expression) return;

    // Free local variables
    EVAL_LOCAL_VARIABLE *var = expression->local_variables;
    while (var) {
        EVAL_LOCAL_VARIABLE *next = var->next;
        string_freez(var->name);
        freez(var);
        var = next;
    }

    if(expression->nodes) eval_node_free(expression->nodes);
    string_freez((void *)expression->source);
    string_freez((void *)expression->parsed_as);
    buffer_free(expression->error_msg);
    freez(expression);
}
