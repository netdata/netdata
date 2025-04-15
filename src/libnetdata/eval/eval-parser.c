// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// Character validation functions for parsing
ALWAYS_INLINE
static bool is_operator_first_symbol_or_space(const char s) {
    return (
        isspace((uint8_t)s) || !s ||
        s == '&' || s == '|' || s == '!' || s == '>' || s == '<' ||
        s == '=' || s == '+' || s == '-' || s == '*' || s == '/' || s == '?');
}

ALWAYS_INLINE
static bool is_valid_after_operator_word(const char s) {
    return isspace((uint8_t)s) || s == '(' || s == '$' || s == '!' || 
           s == '-' || s == '+' || isdigit((uint8_t)s) || !s;
}

ALWAYS_INLINE
static bool is_valid_after_operator_symbol(const char s) {
    return is_valid_after_operator_word(s) || is_operator_first_symbol_or_space(s);
}

ALWAYS_INLINE
static bool is_valid_variable_character(const char s) {
    return !is_operator_first_symbol_or_space(s) && s != ')' && s != '}';
}

// Forward function declarations
static inline EVAL_NODE *parse_full_expression(const char **string, int *error);
static inline EVAL_NODE *parse_one_full_operand(const char **string, int *error);

// ----------------------------------------------------------------------------
// parsing expressions

// skip spaces
ALWAYS_INLINE
static void skip_spaces(const char **string) {
    const char *s = *string;
    while(isspace((uint8_t)*s)) s++;
    *string = s;
}

// ----------------------------------------------------------------------------
// parse operators

ALWAYS_INLINE
static int parse_and(const char **string) {
    const char *s = *string;

    // AND
    if((s[0] == 'A' || s[0] == 'a') && (s[1] == 'N' || s[1] == 'n') && (s[2] == 'D' || s[2] == 'd') &&
        is_valid_after_operator_word(s[3])) {
        *string = &s[4];
        return 1;
    }

    // &&
    if(s[0] == '&' && s[1] == '&' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_or(const char **string) {
    const char *s = *string;

    // OR
    if((s[0] == 'O' || s[0] == 'o') && (s[1] == 'R' || s[1] == 'r') && is_valid_after_operator_word(s[2])) {
        *string = &s[3];
        return 1;
    }

    // ||
    if(s[0] == '|' && s[1] == '|' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_greater_than_or_equal(const char **string) {
    const char *s = *string;

    // >=
    if(s[0] == '>' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_less_than_or_equal(const char **string) {
    const char *s = *string;

    // <=
    if (s[0] == '<' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_greater(const char **string) {
    const char *s = *string;

    // >
    if(s[0] == '>' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_less(const char **string) {
    const char *s = *string;

    // <
    if(s[0] == '<' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_equal(const char **string) {
    const char *s = *string;

    // ==
    if(s[0] == '=' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    // =
    if(s[0] == '=' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_not_equal(const char **string) {
    const char *s = *string;

    // !=
    if(s[0] == '!' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    // <>
    if(s[0] == '<' && s[1] == '>' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
    }

    return 0;
}

ALWAYS_INLINE
static int parse_not(const char **string) {
    const char *s = *string;

    // NOT
    if((s[0] == 'N' || s[0] == 'n') && (s[1] == 'O' || s[1] == 'o') && (s[2] == 'T' || s[2] == 't') &&
        is_valid_after_operator_word(s[3])) {
        *string = &s[3];
        return 1;
    }

    if(s[0] == '!') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_multiply(const char **string) {
    const char *s = *string;

    // *
    if(s[0] == '*' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_divide(const char **string) {
    const char *s = *string;

    // /
    if(s[0] == '/' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_minus(const char **string) {
    const char *s = *string;

    // -
    if(s[0] == '-' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_plus(const char **string) {
    const char *s = *string;

    // +
    if(s[0] == '+' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_open_subexpression(const char **string) {
    const char *s = *string;

    // (
    if(s[0] == '(') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_close_subexpression(const char **string) {
    const char *s = *string;

    // )
    if(s[0] == ')') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_variable(const char **string, char *buffer, size_t len) {
    const char *s = *string;

    // $
    if(*s == '$') {
        size_t i = 0;
        s++;

        if(*s == '{') {
            // ${variable_name}

            s++;
            while (*s && *s != '}' && i < len)
                buffer[i++] = *s++;

            if(*s == '}')
                s++;
        }
        else {
            // $variable_name

            while (*s && is_valid_variable_character(*s) && i < len)
                buffer[i++] = *s++;
        }

        buffer[i] = '\0';

        if (buffer[0]) {
            *string = s;
            return 1;
        }
    }

    return 0;
}

ALWAYS_INLINE
static int parse_constant(const char **string, NETDATA_DOUBLE *number) {
    char *end = NULL;
    NETDATA_DOUBLE n = str2ndd(*string, &end);
    if(unlikely(!end || *string == end)) {
        *number = 0;
        return 0;
    }
    *number = n;
    *string = end;
    return 1;
}

ALWAYS_INLINE
static int parse_abs(const char **string) {
    const char *s = *string;

    // ABS
    if((s[0] == 'A' || s[0] == 'a') && (s[1] == 'B' || s[1] == 'b') && (s[2] == 'S' || s[2] == 's') && s[3] == '(') {
        *string = &s[3];
        return 1;
    }

    return 0;
}

ALWAYS_INLINE
static int parse_if_then_else(const char **string) {
    const char *s = *string;

    // ?
    if(s[0] == '?') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static struct operator_parser {
    unsigned char id;
    int (*parse)(const char **);
} operator_parsers[] = {
        // the order in this list is important!
        // the first matching will be used
        // so place the longer of overlapping ones
        // at the top

        { EVAL_OPERATOR_AND,                   parse_and },
        { EVAL_OPERATOR_OR,                    parse_or },
        { EVAL_OPERATOR_GREATER_THAN_OR_EQUAL, parse_greater_than_or_equal },
        { EVAL_OPERATOR_LESS_THAN_OR_EQUAL,    parse_less_than_or_equal },
        { EVAL_OPERATOR_NOT_EQUAL,             parse_not_equal },
        { EVAL_OPERATOR_EQUAL,                 parse_equal },
        { EVAL_OPERATOR_LESS,                  parse_less },
        { EVAL_OPERATOR_GREATER,               parse_greater },
        { EVAL_OPERATOR_PLUS,                  parse_plus },
        { EVAL_OPERATOR_MINUS,                 parse_minus },
        { EVAL_OPERATOR_MULTIPLY,              parse_multiply },
        { EVAL_OPERATOR_DIVIDE,                parse_divide },
        { EVAL_OPERATOR_IF_THEN_ELSE,          parse_if_then_else },

        /* we should not put in this list the following:
         *
         *  - NOT
         *  - (
         *  - )
         *
         * these are handled in code
         */

        // termination
        { EVAL_OPERATOR_NOP, NULL }
};

ALWAYS_INLINE
static unsigned char parse_operator(const char **string, int *precedence) {
    skip_spaces(string);

    int i;
    for(i = 0 ; operator_parsers[i].parse != NULL ; i++)
        if(operator_parsers[i].parse(string)) {
            if(precedence) *precedence = eval_precedence(operator_parsers[i].id);
            return operator_parsers[i].id;
        }

    return EVAL_OPERATOR_NOP;
}

// ----------------------------------------------------------------------------
// the parsing logic

// helper function to avoid allocations all over the place
ALWAYS_INLINE
static EVAL_NODE *parse_next_operand_given_its_operator(const char **string, unsigned char operator_type, int *error) {
    EVAL_NODE *sub = parse_one_full_operand(string, error);
    if(!sub) return NULL;

    EVAL_NODE *op = eval_node_alloc(1);
    op->operator = operator_type;
    eval_node_set_value_to_node(op, 0, sub);
    return op;
}

// parse a full operand, including its sign or other associative operator (e.g. NOT)
static inline EVAL_NODE *parse_one_full_operand(const char **string, int *error) {
    char variable_buffer[EVAL_MAX_VARIABLE_NAME_LENGTH + 1];
    EVAL_NODE *op1 = NULL;
    NETDATA_DOUBLE number;

    *error = EVAL_ERROR_OK;

    skip_spaces(string);
    if(!(**string)) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NULL;
    }

    if(parse_not(string)) {
        op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_NOT, error);
        op1->precedence = eval_precedence(EVAL_OPERATOR_NOT);
    }
    else if(parse_plus(string)) {
        op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_SIGN_PLUS, error);
        op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_PLUS);
    }
    else if(parse_minus(string)) {
        op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_SIGN_MINUS, error);
        op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_MINUS);
    }
    else if(parse_abs(string)) {
        op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_ABS, error);
        op1->precedence = eval_precedence(EVAL_OPERATOR_ABS);
    }
    else if(parse_open_subexpression(string)) {
        EVAL_NODE *sub = parse_full_expression(string, error);
        if(sub) {
            op1 = eval_node_alloc(1);
            op1->operator = EVAL_OPERATOR_EXPRESSION_OPEN;
            op1->precedence = eval_precedence(EVAL_OPERATOR_EXPRESSION_OPEN);
            eval_node_set_value_to_node(op1, 0, sub);
            if(!parse_close_subexpression(string)) {
                *error = EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION;
                eval_node_free(op1);
                return NULL;
            }
        }
    }
    else if(parse_variable(string, variable_buffer, EVAL_MAX_VARIABLE_NAME_LENGTH)) {
        op1 = eval_node_alloc(1);
        op1->operator = EVAL_OPERATOR_NOP;
        eval_node_set_value_to_variable(op1, 0, variable_buffer);
    }
    else if(parse_constant(string, &number)) {
        op1 = eval_node_alloc(1);
        op1->operator = EVAL_OPERATOR_NOP;
        eval_node_set_value_to_constant(op1, 0, number);
    }
    else if(**string)
        *error = EVAL_ERROR_UNKNOWN_OPERAND;
    else
        *error = EVAL_ERROR_MISSING_OPERAND;

    return op1;
}

// parse an operator and the rest of the expression
// precedence processing is handled here
ALWAYS_INLINE
static EVAL_NODE *parse_rest_of_expression(const char **string, int *error, EVAL_NODE *op1) {
    EVAL_NODE *op2 = NULL;
    unsigned char operator;
    int precedence;

    operator = parse_operator(string, &precedence);
    skip_spaces(string);

    if(operator != EVAL_OPERATOR_NOP) {
        op2 = parse_one_full_operand(string, error);
        if(!op2) {
            // error is already reported
            eval_node_free(op1);
            return NULL;
        }

        EVAL_NODE *op = eval_node_alloc(operators[operator].parameters);
        op->operator = operator;
        op->precedence = precedence;

        if(operator == EVAL_OPERATOR_IF_THEN_ELSE && op->count == 3) {
            skip_spaces(string);

            if(**string != ':') {
                eval_node_free(op);
                eval_node_free(op1);
                eval_node_free(op2);
                *error = EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE;
                return NULL;
            }
            (*string)++;

            skip_spaces(string);

            // For the else part, we need to handle nested ternary operators
            // So we use parse_full_expression instead of parse_one_full_operand
            // This ensures proper parsing of nested ternary operators
            EVAL_NODE *op3 = parse_full_expression(string, error);
            if(!op3) {
                eval_node_free(op);
                eval_node_free(op1);
                eval_node_free(op2);
                // error is already reported
                return NULL;
            }

            eval_node_set_value_to_node(op, 2, op3);
        }

        eval_node_set_value_to_node(op, 1, op2);

        // precedence processing
        // if this operator has a higher precedence compared to its next
        // put the next operator on top of us (top = evaluated later)
        // function recursion does the rest...
        if(op->precedence > op1->precedence && op1->count == 2 && op1->operator != '(' && op1->ops[1].type == EVAL_VALUE_EXPRESSION) {
            eval_node_set_value_to_node(op, 0, op1->ops[1].expression);
            op1->ops[1].expression = op;
            op = op1;
        }
        else
            eval_node_set_value_to_node(op, 0, op1);

        return parse_rest_of_expression(string, error, op);
    }
    else if(**string == ')') {
        ;
    }
    else if(**string) {
        eval_node_free(op1);
        op1 = NULL;
        *error = EVAL_ERROR_MISSING_OPERATOR;
    }

    return op1;
}

// high level function to parse an expression or a sub-expression
static inline EVAL_NODE *parse_full_expression(const char **string, int *error) {
    EVAL_NODE *op1 = parse_one_full_operand(string, error);
    if(!op1) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NULL;
    }

    return parse_rest_of_expression(string, error, op1);
}

// ----------------------------------------------------------------------------
// public API for parsing

EVAL_EXPRESSION *expression_parse(const char *string, const char **failed_at, int *error) {
    if(!string || !*string)
        return NULL;

    const char *s = string;
    int err = EVAL_ERROR_OK;

    EVAL_NODE *op = parse_full_expression(&s, &err);

    if(*s) {
        if(op) {
            eval_node_free(op);
            op = NULL;
        }
        err = EVAL_ERROR_REMAINING_GARBAGE;
    }

    if (failed_at) *failed_at = s;
    if (error) *error = err;

    if(!op) {
        unsigned long pos = s - string + 1;
        netdata_log_error("failed to parse expression '%s': %s at character %lu (i.e.: '%s').", string, expression_strerror(err), pos, s);
        return NULL;
    }

    BUFFER *out = buffer_create(1024, NULL);
    print_parsed_as_node(out, op, &err);
    if(err != EVAL_ERROR_OK) {
        netdata_log_error("failed to re-generate expression '%s' with reason: %s", string, expression_strerror(err));
        eval_node_free(op);
        buffer_free(out);
        return NULL;
    }

    EVAL_EXPRESSION *exp = callocz(1, sizeof(EVAL_EXPRESSION));

    exp->source = string_strdupz(string);
    exp->parsed_as = string_strdupz(buffer_tostring(out));
    buffer_free(out);

    exp->error_msg = buffer_create(100, NULL);
    exp->nodes = op;

    return exp;
}
