// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

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
#define EVAL_OPERATOR_SIGN_PLUS             'P'
#define EVAL_OPERATOR_SIGN_MINUS            'M'
#define EVAL_OPERATOR_ABS                   'A'
#define EVAL_OPERATOR_IF_THEN_ELSE          '?'

// ----------------------------------------------------------------------------
// forward function definitions

static inline void eval_node_free(EVAL_NODE *op);
static inline EVAL_NODE *parse_full_expression(const char **string, int *error);
static inline EVAL_NODE *parse_one_full_operand(const char **string, int *error);
static inline NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error);
static inline void print_parsed_as_node(BUFFER *out, EVAL_NODE *op, int *error);
static inline void print_parsed_as_constant(BUFFER *out, NETDATA_DOUBLE n);

// ----------------------------------------------------------------------------
// evaluation of expressions

static inline NETDATA_DOUBLE eval_variable(EVAL_EXPRESSION *exp, EVAL_VARIABLE *v, int *error) {
    NETDATA_DOUBLE n;

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

static inline NETDATA_DOUBLE eval_value(EVAL_EXPRESSION *exp, EVAL_VALUE *v, int *error) {
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

static inline int is_true(NETDATA_DOUBLE n) {
    if(isnan(n)) return 0;
    if(isinf(n)) return 1;
    if(n == 0) return 0;
    return 1;
}

NETDATA_DOUBLE eval_and(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return is_true(eval_value(exp, &op->ops[0], error)) && is_true(eval_value(exp, &op->ops[1], error));
}
NETDATA_DOUBLE eval_or(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return is_true(eval_value(exp, &op->ops[0], error)) || is_true(eval_value(exp, &op->ops[1], error));
}
NETDATA_DOUBLE eval_greater_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isgreaterequal(n1, n2);
}
NETDATA_DOUBLE eval_less_than_or_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return islessequal(n1, n2);
}
NETDATA_DOUBLE eval_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) && isnan(n2)) return 1;
    if(isinf(n1) && isinf(n2)) return 1;
    if(isnan(n1) || isnan(n2)) return 0;
    if(isinf(n1) || isinf(n2)) return 0;
    return considered_equal_ndd(n1, n2);
}
NETDATA_DOUBLE eval_not_equal(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return !eval_equal(exp, op, error);
}
NETDATA_DOUBLE eval_less(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isless(n1, n2);
}
NETDATA_DOUBLE eval_greater(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    return isgreater(n1, n2);
}
NETDATA_DOUBLE eval_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 + n2;
}
NETDATA_DOUBLE eval_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 - n2;
}
NETDATA_DOUBLE eval_multiply(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 * n2;
}
NETDATA_DOUBLE eval_divide(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    NETDATA_DOUBLE n2 = eval_value(exp, &op->ops[1], error);
    if(isnan(n1) || isnan(n2)) return NAN;
    if(isinf(n1) || isinf(n2)) return INFINITY;
    return n1 / n2;
}
NETDATA_DOUBLE eval_nop(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return eval_value(exp, &op->ops[0], error);
}
NETDATA_DOUBLE eval_not(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return !is_true(eval_value(exp, &op->ops[0], error));
}
NETDATA_DOUBLE eval_sign_plus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    return eval_value(exp, &op->ops[0], error);
}
NETDATA_DOUBLE eval_sign_minus(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(isnan(n1)) return NAN;
    if(isinf(n1)) return INFINITY;
    return -n1;
}
NETDATA_DOUBLE eval_abs(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    NETDATA_DOUBLE n1 = eval_value(exp, &op->ops[0], error);
    if(isnan(n1)) return NAN;
    if(isinf(n1)) return INFINITY;
    return ABS(n1);
}
NETDATA_DOUBLE eval_if_then_else(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    if(is_true(eval_value(exp, &op->ops[0], error)))
        return eval_value(exp, &op->ops[1], error);
    else
        return eval_value(exp, &op->ops[2], error);
}

static struct operator {
    const char *print_as;
    char precedence;
    char parameters;
    char isfunction;
    NETDATA_DOUBLE (*eval)(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error);
} operators[256] = {
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
        [EVAL_OPERATOR_ABS]                   = { "abs(",6,1, 1, eval_abs },
        [EVAL_OPERATOR_IF_THEN_ELSE]          = { "?",  7, 3, 0, eval_if_then_else },
        [EVAL_OPERATOR_NOP]                   = { NULL, 8, 1, 0, eval_nop },
        [EVAL_OPERATOR_EXPRESSION_OPEN]       = { NULL, 8, 1, 0, eval_nop },

        // this should exist in our evaluation list
        [EVAL_OPERATOR_EXPRESSION_CLOSE]      = { NULL, 99, 1, 0, eval_nop }
};

#define eval_precedence(operator) (operators[(unsigned char)(operator)].precedence)

static inline NETDATA_DOUBLE eval_node(EVAL_EXPRESSION *exp, EVAL_NODE *op, int *error) {
    if(unlikely(op->count != operators[op->operator].parameters)) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return 0;
    }

    NETDATA_DOUBLE n = operators[op->operator].eval(exp, op, error);

    return n;
}

// ----------------------------------------------------------------------------
// parsed-as generation

static inline void print_parsed_as_variable(BUFFER *out, EVAL_VARIABLE *v, int *error) {
    (void)error;
    buffer_sprintf(out, "${%s}", string2str(v->name));
}

static inline void print_parsed_as_constant(BUFFER *out, NETDATA_DOUBLE n) {
    if(unlikely(isnan(n))) {
        buffer_strcat(out, "nan");
        return;
    }

    if(unlikely(isinf(n))) {
        buffer_strcat(out, "inf");
        return;
    }

    char b[100+1], *s;
    snprintfz(b, sizeof(b) - 1, NETDATA_DOUBLE_FORMAT, n);

    s = &b[strlen(b) - 1];
    while(s > b && *s == '0') {
        *s ='\0';
        s--;
    }

    if(s > b && *s == '.')
        *s = '\0';

    buffer_strcat(out, b);
}

static inline void print_parsed_as_value(BUFFER *out, EVAL_VALUE *v, int *error) {
    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            print_parsed_as_node(out, v->expression, error);
            break;

        case EVAL_VALUE_NUMBER:
            print_parsed_as_constant(out, v->number);
            break;

        case EVAL_VALUE_VARIABLE:
            print_parsed_as_variable(out, v->variable, error);
            break;

        default:
            *error = EVAL_ERROR_INVALID_VALUE;
            break;
    }
}

static inline void print_parsed_as_node(BUFFER *out, EVAL_NODE *op, int *error) {
    if(unlikely(op->count != operators[op->operator].parameters)) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return;
    }

    if(operators[op->operator].parameters == 1) {

        if(operators[op->operator].print_as)
            buffer_sprintf(out, "%s", operators[op->operator].print_as);

        //if(op->operator == EVAL_OPERATOR_EXPRESSION_OPEN)
        //    buffer_strcat(out, "(");

        print_parsed_as_value(out, &op->ops[0], error);

        //if(op->operator == EVAL_OPERATOR_EXPRESSION_OPEN)
        //    buffer_strcat(out, ")");
    }

    else if(operators[op->operator].parameters == 2) {
        buffer_strcat(out, "(");
        print_parsed_as_value(out, &op->ops[0], error);

        if(operators[op->operator].print_as)
            buffer_sprintf(out, " %s ", operators[op->operator].print_as);

        print_parsed_as_value(out, &op->ops[1], error);
        buffer_strcat(out, ")");
    }
    else if(op->operator == EVAL_OPERATOR_IF_THEN_ELSE && operators[op->operator].parameters == 3) {
        buffer_strcat(out, "(");
        print_parsed_as_value(out, &op->ops[0], error);

        if(operators[op->operator].print_as)
            buffer_sprintf(out, " %s ", operators[op->operator].print_as);

        print_parsed_as_value(out, &op->ops[1], error);
        buffer_strcat(out, " : ");
        print_parsed_as_value(out, &op->ops[2], error);
        buffer_strcat(out, ")");
    }

    if(operators[op->operator].isfunction)
        buffer_strcat(out, ")");
}

// ----------------------------------------------------------------------------
// parsing expressions

// skip spaces
static inline void skip_spaces(const char **string) {
    const char *s = *string;
    while(isspace((uint8_t)*s)) s++;
    *string = s;
}

//static inline int old_isoperatorterm_word(const char s) {
//    if (isspace(s) || s == '(' || s == '$' || s == '!' || s == '-' || s == '+' || isdigit(s) || !s)
//        return 1;
//    return 0;
//}
//
//static inline int old_isoperatorterm_symbol(const char s) {
//    if (old_isoperatorterm_word(s) || isalpha(s))
//        return 1;
//    return 0;
//}
//
//// return 1 if the character should never appear in a variable
//static inline int old_isvariableterm(const char s) {
//    if (isalnum(s) || s == '.' || s == '_')
//        return 0;
//    return 1;
//}

static inline bool is_operator_first_symbol_or_space(const char s) {
    return (
        isspace((uint8_t)s) || !s ||
        s == '&' || s == '|' || s == '!' || s == '>' || s == '<' ||
        s == '=' || s == '+' || s == '-' || s == '*' || s == '/' || s == '?');
}

// what character can appear just after the operators: NOT, AND, OR
static inline bool is_valid_after_operator_word(const char s) {
    bool rc = isspace((uint8_t)s) || s == '(' || s == '$' || s == '!' || s == '-' || s == '+' || isdigit((uint8_t)s) || !s;
//    bool old = old_isoperatorterm_word(s);
//    if(rc != old) {
//        int x = 0;
//        x++;
//    }
    return rc;
}

// what character can appear just after an operator symbol?
static inline bool is_valid_after_operator_symbol(const char s) {
    bool rc = is_valid_after_operator_word(s) || is_operator_first_symbol_or_space(s);
//    bool old = old_isoperatorterm_symbol(s);
//    if(rc != old) {
//        int x = 0;
//        x++;
//    }
    return rc;
}

// return true if the character may appear in a variable name
static inline bool is_valid_variable_character(const char s) {
    bool rc = !is_operator_first_symbol_or_space(s) && s != ')' && s != '}';
//    bool old = !old_isvariableterm(s);
//    if(rc != old) {
//        int x = 0;
//        x++;
//    }
    return rc;
}

// ----------------------------------------------------------------------------
// parse operators

static inline int parse_and(const char **string) {
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

static inline int parse_or(const char **string) {
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

static inline int parse_greater_than_or_equal(const char **string) {
    const char *s = *string;

    // >=
    if(s[0] == '>' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_less_than_or_equal(const char **string) {
    const char *s = *string;

    // <=
    if (s[0] == '<' && s[1] == '=' && is_valid_after_operator_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_greater(const char **string) {
    const char *s = *string;

    // >
    if(s[0] == '>' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_less(const char **string) {
    const char *s = *string;

    // <
    if(s[0] == '<' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_equal(const char **string) {
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

static inline int parse_not_equal(const char **string) {
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

static inline int parse_not(const char **string) {
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

static inline int parse_multiply(const char **string) {
    const char *s = *string;

    // *
    if(s[0] == '*' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_divide(const char **string) {
    const char *s = *string;

    // /
    if(s[0] == '/' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_minus(const char **string) {
    const char *s = *string;

    // -
    if(s[0] == '-' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_plus(const char **string) {
    const char *s = *string;

    // +
    if(s[0] == '+' && is_valid_after_operator_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_open_subexpression(const char **string) {
    const char *s = *string;

    // (
    if(s[0] == '(') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

#define parse_close_function(x) parse_close_subexpression(x)

static inline int parse_close_subexpression(const char **string) {
    const char *s = *string;

    // )
    if(s[0] == ')') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_variable(const char **string, char *buffer, size_t len) {
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

static inline int parse_constant(const char **string, NETDATA_DOUBLE *number) {
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

static inline int parse_abs(const char **string) {
    const char *s = *string;

    // ABS
    if((s[0] == 'A' || s[0] == 'a') && (s[1] == 'B' || s[1] == 'b') && (s[2] == 'S' || s[2] == 's') && s[3] == '(') {
        *string = &s[3];
        return 1;
    }

    return 0;
}

static inline int parse_if_then_else(const char **string) {
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

static inline unsigned char parse_operator(const char **string, int *precedence) {
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
// memory management

static inline EVAL_NODE *eval_node_alloc(int count) {
    static int id = 1;

    EVAL_NODE *op = callocz(1, sizeof(EVAL_NODE) + (sizeof(EVAL_VALUE) * count));

    op->id = id++;
    op->operator = EVAL_OPERATOR_NOP;
    op->precedence = eval_precedence(EVAL_OPERATOR_NOP);
    op->count = count;
    return op;
}

static inline void eval_node_set_value_to_node(EVAL_NODE *op, int pos, EVAL_NODE *value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_EXPRESSION;
    op->ops[pos].expression = value;
}

static inline void eval_node_set_value_to_constant(EVAL_NODE *op, int pos, NETDATA_DOUBLE value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_NUMBER;
    op->ops[pos].number = value;
}

static inline void eval_node_set_value_to_variable(EVAL_NODE *op, int pos, const char *variable) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_VARIABLE;
    op->ops[pos].variable = callocz(1, sizeof(EVAL_VARIABLE));
    op->ops[pos].variable->name = string_strdupz(variable);
}

static inline void eval_variable_free(EVAL_VARIABLE *v) {
    string_freez(v->name);
    freez(v);
}

static inline void eval_value_free(EVAL_VALUE *v) {
    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            eval_node_free(v->expression);
            break;

        case EVAL_VALUE_VARIABLE:
            eval_variable_free(v->variable);
            break;

        default:
            break;
    }
}

static inline void eval_node_free(EVAL_NODE *op) {
    if(op->count) {
        int i;
        for(i = op->count - 1; i >= 0 ;i--)
            eval_value_free(&op->ops[i]);
    }

    freez(op);
}

// ----------------------------------------------------------------------------
// the parsing logic

// helper function to avoid allocations all over the place
static inline EVAL_NODE *parse_next_operand_given_its_operator(const char **string, unsigned char operator_type, int *error) {
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
static inline EVAL_NODE *parse_rest_of_expression(const char **string, int *error, EVAL_NODE *op1) {
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

            EVAL_NODE *op3 = parse_one_full_operand(string, error);
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
// public API

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

void expression_free(EVAL_EXPRESSION *expression) {
    if(!expression) return;

    if(expression->nodes) eval_node_free(expression->nodes);
    string_freez((void *)expression->source);
    string_freez((void *)expression->parsed_as);
    buffer_free(expression->error_msg);
    freez(expression);
}

const char *expression_strerror(int error) {
    switch(error) {
        case EVAL_ERROR_OK:
            return "success";

        case EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION:
            return "missing closing parenthesis";

        case EVAL_ERROR_UNKNOWN_OPERAND:
            return "unknown operand";

        case EVAL_ERROR_MISSING_OPERAND:
            return "expected operand";

        case EVAL_ERROR_MISSING_OPERATOR:
            return "expected operator";

        case EVAL_ERROR_REMAINING_GARBAGE:
            return "remaining characters after expression";

        case EVAL_ERROR_INVALID_VALUE:
            return "invalid value structure - internal error";

        case EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS:
            return "wrong number of operands for operation - internal error";

        case EVAL_ERROR_VALUE_IS_NAN:
            return "value is unset";

        case EVAL_ERROR_VALUE_IS_INFINITE:
            return "computed value is infinite";

        case EVAL_ERROR_UNKNOWN_VARIABLE:
            return "undefined variable";

        case EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE:
            return "missing second sub-expression of inline conditional";

        default:
            return "unknown error";
    }
}

const char *expression_source(EVAL_EXPRESSION *expression) {
    if(!expression)
        return string2str(NULL);

    return string2str(expression->source);
}

const char *expression_parsed_as(EVAL_EXPRESSION *expression) {
    if(!expression)
        return string2str(NULL);

    return string2str(expression->parsed_as);
}

const char *expression_error_msg(EVAL_EXPRESSION *expression) {
    if(!expression || !expression->error_msg)
        return "";

    return buffer_tostring(expression->error_msg);
}

NETDATA_DOUBLE expression_result(EVAL_EXPRESSION *expression) {
    if(!expression)
        return NAN;

    return expression->result;
}

void expression_set_variable_lookup_callback(EVAL_EXPRESSION *expression, eval_expression_variable_lookup_t cb, void *data) {
    if(!expression)
        return;

    expression->variable_lookup_cb = cb;
    expression->variable_lookup_cb_data = data;
}

static size_t expression_hardcode_node_variable(EVAL_NODE *node, STRING *variable, NETDATA_DOUBLE value) {
    size_t matches = 0;

    for(int i = 0; i < node->count; i++) {
        switch(node->ops[i].type) {
            case EVAL_VALUE_NUMBER:
            case EVAL_VALUE_INVALID:
                break;

            case EVAL_VALUE_VARIABLE:
                if(node->ops[i].variable->name == variable) {
                    string_freez(node->ops[i].variable->name);
                    freez(node->ops[i].variable);
                    node->ops[i].type = EVAL_VALUE_NUMBER;
                    node->ops[i].number = value;
                    matches++;
                }
                break;

            case EVAL_VALUE_EXPRESSION:
                matches += expression_hardcode_node_variable(node->ops[i].expression, variable, value);
                break;
        }
    }

    return matches;
}

void expression_hardcode_variable(EVAL_EXPRESSION *expression, STRING *variable, NETDATA_DOUBLE value) {
    if (!expression || !variable || isnan(value))
        return;

    size_t matches = expression_hardcode_node_variable(expression->nodes, variable, value);
    if (matches) {
        char replace[1024];
        snprintfz(replace, sizeof(replace), NETDATA_DOUBLE_FORMAT_AUTO, value);
        size_t replace_len = strlen(replace);

        size_t source_len = string_strlen(expression->source);
        const char *source_str = string2str(expression->source);

        // Allocate enough space to accommodate all replacements.
        char buf[source_len + 1 + matches * (replace_len + 1)];

        char find1[string_strlen(variable) + 1 + 1];
        snprintfz(find1, sizeof(find1), "$%s", string2str(variable));
        size_t find1_len = strlen(find1);

        char find2[string_strlen(variable) + 1 + 3];
        snprintfz(find2, sizeof(find2), "${%s}", string2str(variable));
        size_t find2_len = strlen(find2);

        size_t found = 0;
        char *buf_ptr = buf;
        const char *source_ptr = source_str;

        while (*source_ptr) {
            char *s1 = strstr(source_ptr, find1);
            char *s2 = strstr(source_ptr, find2);

            char *s = s1;
            size_t len = find1_len;
            if (s2 && (!s1 || s2 < s1)) {
                s = s2;
                len = find2_len;
            }

            if (s) {
                if (s == s1 && is_valid_variable_character(s[len])) {
                    // Move past the variable if it's part of a larger word.
                    source_ptr = s + len;
                    continue;
                }

                // Copy the part before the variable.
                memcpy(buf_ptr, source_ptr, s - source_ptr);
                buf_ptr += (s - source_ptr);

                // Copy the replacement.
                memcpy(buf_ptr, replace, replace_len);
                buf_ptr += replace_len;
                *buf_ptr = '\0';

                // Move the source pointer past the replaced variable.
                source_ptr = s + len;
                found++;
            } else {
                // Copy the rest of the string if no more variables are found.
                strcpy(buf_ptr, source_ptr);
                break;
            }
        }

        // Update the expression source with the new string.
        string_freez(expression->source);
        expression->source = string_strdupz(buf);
    }
}
