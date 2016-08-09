#include "common.h"

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
    if((s[0] == 'O' || s[0] == '0') && (s[1] == 'R' || s[1] == 'r') && isoperatorterm_word(s[2])) {
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

// ----------------------------------------------------------------------------
// operators that affect a single operand

static inline int parse_not(const char **string) {
    const char *s = *string;

    // NOT
    if((s[0] == 'N' || s[0] == 'n') && (s[1] == 'O' || s[1] == 'o') && (s[2] == 'T' || s[2] == 't') && isoperatorterm_word(s[3])) {
        *string = &s[4];
        return 1;
    }

    if(s[0] == EVAL_OPERATOR_NOT) {
        *string = &s[1];
        return 1;
    }

    return 0;
}

static struct operator {
    const char *printas;
    int precedence;
    char id;
    int (*parse)(const char **);
} operators[] = {
        { "&&", 2, '&', parse_and },
        { "||", 2, '|', parse_or },
        { ">=", 3, '}', parse_greater_than_or_equal },
        { "<=", 3, '{', parse_less_than_or_equal },
        { "<>", 3, '~', parse_not_equal },
        { "==", 3, '=', parse_equal },
        { "<",  3, '<', parse_less },
        { ">",  3, '>', parse_greater },
        { "+",  4, EVAL_OPERATOR_PLUS, parse_plus },
        { "-",  4, EVAL_OPERATOR_MINUS, parse_minus },
        { "*",  5, '*', parse_multiply },
        { "/",  5, '/', parse_divide },

        // we should not put NOT in this list

        { NULL, 0, EVAL_OPERATOR_NOP, NULL }
};

static inline char parse_operator(const char **s, int *precedence) {
    int i;

    for(i = 0 ; operators[i].parse != NULL ; i++)
        if(operators[i].parse(s)) {
            if(precedence) *precedence = operators[i].precedence;
            return operators[i].id;
        }

    return EVAL_OPERATOR_NOP;
}

static inline EVAL_OPERAND *operand_alloc(int count) {
    EVAL_OPERAND *op = calloc(1, sizeof(EVAL_OPERAND) + (sizeof(EVAL_VALUE) * count));
    if(!op) fatal("Cannot allocate memory for OPERAND");

    op->count = count;
    return op;
}

static inline void operand_set_value_operand(EVAL_OPERAND *op, int pos, EVAL_OPERAND *value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_OPERAND_EXPRESSION;
    op->ops[pos].expression = value;
}

// forward definitions
static inline EVAL_OPERAND *parse_operand(const char **string);

static inline EVAL_OPERAND *operand_alloc_single(const char **string, char type) {
    EVAL_OPERAND *sub = parse_operand(string);
    if(!sub) return NULL;

    EVAL_OPERAND *op = operand_alloc(1);
    if(!op) fatal("Cannot allocate memory for OPERAND");

    op->operator = type;
    operand_set_value_operand(op, 0, sub);
    return op;
}

static inline EVAL_OPERAND *parse_operand(const char **string) {
    const char *s = *string;
    while(isspace(*s)) s++;

    if(!*s) return NULL;
    *string = s;

    if(parse_not(string))
        return operand_alloc_single(string, EVAL_OPERATOR_NOT);

    if(parse_plus(string))
        return operand_alloc_single(string, EVAL_OPERATOR_PLUS);

    if(parse_minus(string))
        return operand_alloc_single(string, EVAL_OPERATOR_MINUS);



    return NULL;
}
