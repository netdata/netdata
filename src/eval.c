#include "common.h"

// forward definitions
static inline void eval_node_free(EVAL_NODE *op);
static inline EVAL_NODE *parse_full_expression(const char **string, int *error);
static inline EVAL_NODE *parse_one_full_operand(const char **string, int *error);
static inline calculated_number eval_node(EVAL_NODE *op, int *error);

static inline void skip_spaces(const char **string) {
    const char *s = *string;
    while(isspace(*s)) s++;
    *string = s;
}

// ----------------------------------------------------------------------------
// operators that work on 2 operands

static inline int isoperatorterm_word(const char s) {
    if(isspace(s) || s == '(' || s == '$' || s == '!' || s == '-' || s == '+' || isdigit(s)) return 1;
    return 0;
}

static inline int isoperatorterm_symbol(const char s) {
    if(isoperatorterm_word(s) || isalpha(s)) return 1;
    return 0;
}

static inline int parse_and(const char **string) {
    const char *s = *string;

    // AND
    if((s[0] == 'A' || s[0] == 'a') && (s[1] == 'N' || s[1] == 'n') && (s[2] == 'D' || s[2] == 'd') && isoperatorterm_word(s[3])) {
        *string = &s[4];
        return 1;
    }

    // &&
    if(s[0] == '&' && s[1] == '&' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_or(const char **string) {
    const char *s = *string;

    // OR
    if((s[0] == 'O' || s[0] == 'o') && (s[1] == 'R' || s[1] == 'r') && isoperatorterm_word(s[2])) {
        *string = &s[3];
        return 1;
    }

    // ||
    if(s[0] == '|' && s[1] == '|' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_greater_than_or_equal(const char **string) {
    const char *s = *string;

    // >=
    if(s[0] == '>' && s[1] == '=' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_less_than_or_equal(const char **string) {
    const char *s = *string;

    // <=
    if (s[0] == '<' && s[1] == '=' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    return 0;
}

static inline int parse_greater(const char **string) {
    const char *s = *string;

    // >
    if(s[0] == '>' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_less(const char **string) {
    const char *s = *string;

    // <
    if(s[0] == '<' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_equal(const char **string) {
    const char *s = *string;

    // ==
    if(s[0] == '=' && s[1] == '=' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    // =
    if(s[0] == '=' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_not_equal(const char **string) {
    const char *s = *string;

    // !=
    if(s[0] == '!' && s[1] == '=' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
        return 1;
    }

    // <>
    if(s[0] == '<' && s[1] == '>' && isoperatorterm_symbol(s[2])) {
        *string = &s[2];
    }

    return 0;
}

static inline int parse_multiply(const char **string) {
    const char *s = *string;

    // *
    if(s[0] == '*' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_divide(const char **string) {
    const char *s = *string;

    // /
    if(s[0] == '/' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_minus(const char **string) {
    const char *s = *string;

    // -
    if(s[0] == '-' && isoperatorterm_symbol(s[1])) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_plus(const char **string) {
    const char *s = *string;

    // +
    if(s[0] == '+' && isoperatorterm_symbol(s[1])) {
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

static inline int parse_close_subexpression(const char **string) {
    const char *s = *string;

    // (
    if(s[0] == ')') {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static inline int parse_constant(const char **string, calculated_number *number) {
    char *end = NULL;
    calculated_number n = strtold(*string, &end);
    if(unlikely(!end || *string == end || isnan(n) || isinf(n))) {
        *number = 0;
        return 0;
    }
    *number = n;
    *string = end;
    return 1;
}

// ----------------------------------------------------------------------------
// operators that affect a single operand

static inline int parse_not(const char **string) {
    const char *s = *string;

    // NOT
    if((s[0] == 'N' || s[0] == 'n') && (s[1] == 'O' || s[1] == 'o') && (s[2] == 'T' || s[2] == 't') && isoperatorterm_word(s[3])) {
        *string = &s[3];
        return 1;
    }

    if(s[0] == EVAL_OPERATOR_NOT) {
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

        /* we should not put in this list the following:
         *
         *  - NOT
         *  - (
         *  - )
         *
         * these are handled in code
         */

        { EVAL_OPERATOR_NOP, NULL }
};

// ----------------------------------------------------------------------------
// evaluation of operations

static inline calculated_number eval_check_number(calculated_number n, int *error) {
    if(unlikely(isnan(n))) {
        *error = EVAL_ERROR_VALUE_IS_NAN;
        return 0;
    }

    if(unlikely(isinf(n))) {
        *error = EVAL_ERROR_VALUE_IS_INFINITE;
        return 0;
    }

    return n;
}

static inline calculated_number eval_value(EVAL_VALUE *v, int *error) {
    calculated_number n;

    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            n = eval_node(v->expression, error);
            break;

        case EVAL_VALUE_NUMBER:
            n = v->number;
            break;

//        case EVAL_VALUE_VARIABLE:
//            break;

        default:
            *error = EVAL_ERROR_INVALID_VALUE;
            n = 0;
            break;
    }

    return eval_check_number(n, error);
}

calculated_number eval_and(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) && eval_value(&op->ops[1], error);
}
calculated_number eval_or(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) || eval_value(&op->ops[1], error);
}
calculated_number eval_greater_than_or_equal(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) >= eval_value(&op->ops[1], error);
}
calculated_number eval_less_than_or_equal(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) <= eval_value(&op->ops[1], error);
}
calculated_number eval_not_equal(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) != eval_value(&op->ops[1], error);
}
calculated_number eval_equal(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) == eval_value(&op->ops[1], error);
}
calculated_number eval_less(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) < eval_value(&op->ops[1], error);
}
calculated_number eval_greater(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) > eval_value(&op->ops[1], error);
}
calculated_number eval_plus(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) + eval_value(&op->ops[1], error);
}
calculated_number eval_minus(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) - eval_value(&op->ops[1], error);
}
calculated_number eval_multiply(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) * eval_value(&op->ops[1], error);
}
calculated_number eval_divide(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error) / eval_value(&op->ops[1], error);
}
calculated_number eval_nop(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error);
}
calculated_number eval_not(EVAL_NODE *op, int *error) {
    return !eval_value(&op->ops[0], error);
}
calculated_number eval_sign_plus(EVAL_NODE *op, int *error) {
    return eval_value(&op->ops[0], error);
}
calculated_number eval_sign_minus(EVAL_NODE *op, int *error) {
    return -eval_value(&op->ops[0], error);
}

static struct operator {
    const char *print_as;
    char precedence;
    char parameters;
    calculated_number (*eval)(EVAL_NODE *op, int *error);
} operators[256] = {
        // this is a random access array
        // we always access it with a known EVAL_OPERATOR_X

        [EVAL_OPERATOR_AND]                   = { "&&", 2, 2, eval_and },
        [EVAL_OPERATOR_OR]                    = { "||", 2, 2, eval_or },
        [EVAL_OPERATOR_GREATER_THAN_OR_EQUAL] = { ">=", 3, 2, eval_greater_than_or_equal },
        [EVAL_OPERATOR_LESS_THAN_OR_EQUAL]    = { "<=", 3, 2, eval_less_than_or_equal },
        [EVAL_OPERATOR_NOT_EQUAL]             = { "!=", 3, 2, eval_not_equal },
        [EVAL_OPERATOR_EQUAL]                 = { "==", 3, 2, eval_equal },
        [EVAL_OPERATOR_LESS]                  = { "<",  3, 2, eval_less },
        [EVAL_OPERATOR_GREATER]               = { ">",  3, 2, eval_greater },
        [EVAL_OPERATOR_PLUS]                  = { "+",  4, 2, eval_plus },
        [EVAL_OPERATOR_MINUS]                 = { "-",  4, 2, eval_minus },
        [EVAL_OPERATOR_MULTIPLY]              = { "*",  5, 2, eval_multiply },
        [EVAL_OPERATOR_DIVIDE]                = { "/",  5, 2, eval_divide },
        [EVAL_OPERATOR_NOT]                   = { "!",  6, 1, eval_not },
        [EVAL_OPERATOR_SIGN_PLUS]             = { "+",  6, 1, eval_sign_plus },
        [EVAL_OPERATOR_SIGN_MINUS]            = { "-",  6, 1, eval_sign_minus },
        [EVAL_OPERATOR_NOP]                   = { NULL, 7, 1, eval_nop },
        [EVAL_OPERATOR_VALUE]                 = { NULL, 7, 1, eval_nop },
        [EVAL_OPERATOR_EXPRESSION_OPEN]       = { "(",  7, 1, eval_nop },

        // this should exist in our evaluation list
        [EVAL_OPERATOR_EXPRESSION_CLOSE]      = { ")",  7, 1, eval_nop }
};

#define eval_precedence(operator) (operators[(unsigned char)(operator)].precedence)

static inline calculated_number eval_node(EVAL_NODE *op, int *error) {
    if(unlikely(op->count != operators[op->operator].parameters)) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return 0;
    }

    calculated_number n = operators[op->operator].eval(op, error);

    return eval_check_number(n, error);
}

// ----------------------------------------------------------------------------

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

static inline EVAL_NODE *eval_node_alloc(int count) {
    static int id = 1;

    EVAL_NODE *op = calloc(1, sizeof(EVAL_NODE) + (sizeof(EVAL_VALUE) * count));
    if(!op) fatal("Cannot allocate memory for OPERAND with %d slots", count);

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

static inline void node_set_value_to_constant(EVAL_NODE *op, int pos, calculated_number value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_NUMBER;
    op->ops[pos].number = value;
}

static inline void variable_free(VARIABLE *v) {
    free(v);
}

static inline void value_free(EVAL_VALUE *v) {
    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            eval_node_free(v->expression);
            break;

//        case EVAL_VALUE_VARIABLE:
//            variable_free(v->variable);
//            break;

        default:
            break;
    }
}

static inline void eval_node_free(EVAL_NODE *op) {
    if(op->count) {
        int i;
        for(i = op->count - 1; i >= 0 ;i--)
            value_free(&op->ops[i]);
    }

    free(op);
}

static inline EVAL_NODE *parse_next_operand_given_its_operator(const char **string, unsigned char operator_type, int *error) {
    EVAL_NODE *sub = parse_one_full_operand(string, error);
    if(!sub) return NULL;

    EVAL_NODE *op = eval_node_alloc(1);
    op->operator = operator_type;
    eval_node_set_value_to_node(op, 0, sub);
    return op;
}

static inline EVAL_NODE *parse_one_full_operand(const char **string, int *error) {
    EVAL_NODE *op1 = NULL;
    calculated_number number;

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
//    else if(parse_variable(string)) {
//
//    }
    else if(parse_constant(string, &number)) {
        op1 = eval_node_alloc(1);
        op1->operator = EVAL_OPERATOR_VALUE;
        node_set_value_to_constant(op1, 0, number);
    }
    else if(*string)
        *error = EVAL_ERROR_UNKNOWN_OPERAND;
    else
        *error = EVAL_ERROR_MISSING_OPERAND;

    return op1;
}

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

        EVAL_NODE *op = eval_node_alloc(2);
        op->operator = operator;
        op->precedence = precedence;

        eval_node_set_value_to_node(op, 0, op1);
        eval_node_set_value_to_node(op, 1, op2);

        if(op->precedence > op1->precedence && op1->count == 2 && op1->operator != '(' && op1->ops[1].type == EVAL_VALUE_EXPRESSION) {
            eval_node_set_value_to_node(op, 0, op1->ops[1].expression);
            op1->ops[1].expression = op;
            op = op1;
        }

        return parse_rest_of_expression(string, error, op);
    }
    else if(**string == EVAL_OPERATOR_EXPRESSION_CLOSE) {
        ;
    }
    else if(**string) {
        if(op1) eval_node_free(op1);
        op1 = NULL;
        *error = EVAL_ERROR_MISSING_OPERATOR;
    }

    return op1;
}

static inline EVAL_NODE *parse_full_expression(const char **string, int *error) {
    EVAL_NODE *op1 = NULL;

    op1 = parse_one_full_operand(string, error);
    if(!op1) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NULL;
    }

    return parse_rest_of_expression(string, error, op1);
}

// ----------------------------------------------------------------------------
// public API

EVAL_NODE *expression_parse(const char *string, const char **failed_at, int *error) {
    const char *s;
    int err = EVAL_ERROR_OK;
    unsigned long pos = 0;

    s = string;
    EVAL_NODE *op = parse_full_expression(&s, &err);

    if (failed_at) *failed_at = s;
    if (error) *error = err;

    if(!op) {
        pos = s - string + 1;
        error("failed to parse expression '%s': %s at character %lu (i.e.: '%s').", string, expression_strerror(err), pos, s);
    }

    return op;
}

calculated_number expression_evaluate(EVAL_NODE *expression, int *error) {
    int err = EVAL_ERROR_OK;
    calculated_number ret = eval_node(expression, &err);

    if(err != EVAL_ERROR_OK) {
        error("Failed to execute expression with error %d (%s)", err, expression_strerror(err));
        if(error) *error = err;
        return 0;
    }

    return ret;
}

void expression_free(EVAL_NODE *op) {
    if(op) eval_node_free(op);
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

        case EVAL_ERROR_INVALID_VALUE:
            return "invalid value structure - internal error";

        case EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS:
            return "wrong number of operands for operation - internal error";

        case EVAL_ERROR_VALUE_IS_NAN:
            return "value or variable is missing or is not a number";

        case EVAL_ERROR_VALUE_IS_INFINITE:
            return "computed value is infinite";

        default:
            return "unknown error";
    }
}
