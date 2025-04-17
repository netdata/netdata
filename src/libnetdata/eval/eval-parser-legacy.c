// SPDX-License-Identifier: GPL-3.0-or-later

/*
 * THIS CODE IS NOT USED ANY MORE
 * IT IS KEPT HERE ONLY FOR REFERENCE (AND POTENTIALLY RUNNING UNIT TESTS)
 */

#include "../libnetdata.h"
#include "eval-internal.h"
#include <ctype.h> // For tolower

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
static inline EVAL_NODE *parse_full_expression(const char **string, EVAL_ERROR *error);
static inline EVAL_NODE *parse_one_full_operand(const char **string, EVAL_ERROR *error);

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

// Define the functions we support
static EVAL_FUNCTION eval_functions[] = {
    {"abs", EVAL_OPERATOR_ABS, 6},
    {NULL, 0, 0}  // Terminator
};

// Parse function call
ALWAYS_INLINE
static int parse_function(const char **string, EVAL_OPERATOR *op, int *precedence) {
    const char *s = *string;
    skip_spaces(&s);
    
    // Check for each function in our list
    for (int i = 0; eval_functions[i].name != NULL; i++) {
        const char *name = eval_functions[i].name;
        int len = strlen(name);
        int j;
        
        // Case-insensitive comparison of function name
        for (j = 0; j < len; j++) {
            if (!s[j] || (tolower((unsigned char)s[j]) != name[j])) {
                break;
            }
        }
        
        // Check if we matched the entire function name and it's followed by '('
        if (j == len && s[j] == '(') {
            *string = &s[j+1]; // Move past "function_name("
            *op = eval_functions[i].op;
            if (precedence) *precedence = eval_functions[i].precedence;
            return 1;
        }
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
    EVAL_OPERATOR id;
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
static EVAL_OPERATOR parse_operator(const char **string, int *precedence) {
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

// Forward declarations needed for recursive parsing
static inline EVAL_NODE *parse_expression(const char **string, EVAL_ERROR *error, int allow_functions);
static int starts_with_function(const char *s);

// Helper function to parse a function call
static EVAL_NODE *parse_function_call(const char **string, EVAL_ERROR *error) {
    EVAL_OPERATOR op_type;
    int precedence;
    
    // Parse the function name and opening parenthesis
    if (!parse_function(string, &op_type, &precedence)) {
        *error = EVAL_ERROR_UNKNOWN_OPERAND;
        return NULL;
    }
    
    // Special handling for nested expressions that may include unary operators
    // followed by function calls
    const char *arg_start = *string;
    skip_spaces(&arg_start);
    
    // Check if what follows inside the function's argument is a unary operator followed by another function
    if (arg_start[0] == '-' || arg_start[0] == '+' || arg_start[0] == '!') {
        // Move past this operator character
        arg_start++;
        skip_spaces(&arg_start);
        
        // If what follows is a function call, we need special handling
        if (starts_with_function(arg_start)) {
            // Go back to normal parsing at the start of function arguments
            // and use parse_expression which will handle unary operators correctly
            EVAL_NODE *func_arg = parse_expression(string, error, 1);
            if (!func_arg) {
                *error = EVAL_ERROR_MISSING_OPERAND;
                return NULL;
            }
            
            // Skip the closing parenthesis
            if (!parse_close_subexpression(string)) {
                *error = EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION;
                eval_node_free(func_arg);
                return NULL;
            }
            
            // Create the function node
            EVAL_NODE *func_node = eval_node_alloc(1);
            func_node->operator = op_type;
            func_node->precedence = precedence;
            eval_node_set_value_to_node(func_node, 0, func_arg);
            
            return func_node;
        }
    }
    
    // Regular parsing for function arguments
    EVAL_NODE *func_arg = parse_full_expression(string, error);
    if (!func_arg) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NULL;
    }
    
    // Skip the closing parenthesis
    if (!parse_close_subexpression(string)) {
        *error = EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION;
        eval_node_free(func_arg);
        return NULL;
    }
    
    // Create the function node
    EVAL_NODE *func_node = eval_node_alloc(1);
    func_node->operator = op_type;
    func_node->precedence = precedence;
    eval_node_set_value_to_node(func_node, 0, func_arg);
    
    return func_node;
}

// Helper function to check if a string starts with a function name
static int starts_with_function(const char *s) {
    for (int i = 0; eval_functions[i].name != NULL; i++) {
        const char *name = eval_functions[i].name;
        int len = strlen(name);
        int j;
        
        // Case-insensitive comparison of function name
        for (j = 0; j < len; j++) {
            if (!s[j] || (tolower((unsigned char)s[j]) != name[j])) {
                break;
            }
        }
        
        // Check if we matched the entire function name and it's followed by '('
        if (j == len && s[j] == '(') {
            return 1;
        }
    }
    
    return 0;
}

// Helper function to avoid allocations all over the place
static EVAL_NODE *parse_next_operand_given_its_operator(const char **string, EVAL_OPERATOR operator_type, EVAL_ERROR *error) {
    // Save current position to check for function calls
    const char *current_pos = *string;
    skip_spaces(&current_pos);
    
    // Check if what follows is a function call
    if (starts_with_function(current_pos)) {
        // This is a function - we need special handling
        
        // Parse the function call
        EVAL_NODE *func_node = parse_function_call(&current_pos, error);
        if (!func_node) {
            return NULL;
        }
        
        // Create the unary operator node
        EVAL_NODE *op = eval_node_alloc(1);
        op->operator = operator_type;
        op->precedence = eval_precedence(operator_type);
        eval_node_set_value_to_node(op, 0, func_node);
        
        // Update the string position
        *string = current_pos;
        
        return op;
    }
    
    // Standard parsing for normal operands
    EVAL_NODE *sub = parse_one_full_operand(string, error);
    if(!sub) return NULL;

    EVAL_NODE *op = eval_node_alloc(1);
    op->operator = operator_type;
    eval_node_set_value_to_node(op, 0, sub);
    return op;
}

// parse a full operand, including its sign or other associative operator (e.g. NOT)
static EVAL_NODE *parse_one_full_operand(const char **string, EVAL_ERROR *error) {
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
        // Check if what follows is a function
        skip_spaces(string);
        if (starts_with_function(*string)) {
            // Special case: !function_call()
            EVAL_NODE *func_node = parse_function_call(string, error);
            if (!func_node) return NULL;
            
            op1 = eval_node_alloc(1);
            op1->operator = EVAL_OPERATOR_NOT;
            op1->precedence = eval_precedence(EVAL_OPERATOR_NOT);
            eval_node_set_value_to_node(op1, 0, func_node);
        } else {
            op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_NOT, error);
            if (op1) op1->precedence = eval_precedence(EVAL_OPERATOR_NOT);
        }
    }
    else if(parse_plus(string)) {
        // Check if what follows is a function
        skip_spaces(string);
        if (starts_with_function(*string)) {
            // Special case: +function_call()
            EVAL_NODE *func_node = parse_function_call(string, error);
            if (!func_node) return NULL;
            
            op1 = eval_node_alloc(1);
            op1->operator = EVAL_OPERATOR_SIGN_PLUS;
            op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_PLUS);
            eval_node_set_value_to_node(op1, 0, func_node);
        } else {
            op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_SIGN_PLUS, error);
            if (op1) op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_PLUS);
        }
    }
    else if(parse_minus(string)) {
        // Check if what follows is a function
        skip_spaces(string);
        if (starts_with_function(*string)) {
            // Special case: -function_call()
            EVAL_NODE *func_node = parse_function_call(string, error);
            if (!func_node) return NULL;
            
            op1 = eval_node_alloc(1);
            op1->operator = EVAL_OPERATOR_SIGN_MINUS;
            op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_MINUS);
            eval_node_set_value_to_node(op1, 0, func_node);
        } else {
            op1 = parse_next_operand_given_its_operator(string, EVAL_OPERATOR_SIGN_MINUS, error);
            if (op1) op1->precedence = eval_precedence(EVAL_OPERATOR_SIGN_MINUS);
        }
    }
    else if (starts_with_function(*string)) {
        // Handle the function call
        op1 = parse_function_call(string, error);
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
static EVAL_NODE *parse_rest_of_expression(const char **string, EVAL_ERROR *error, EVAL_NODE *op1) {
    EVAL_NODE *op2 = NULL;
    EVAL_OPERATOR operator;
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

// Parse an expression with optional function support
static inline EVAL_NODE *parse_expression(const char **string, EVAL_ERROR *error, int allow_functions) {
    // Special handling for functions as arguments
    if (allow_functions) {
        const char *s = *string;
        skip_spaces(&s);
        
        // Check for unary operators
        if (s[0] == '-' || s[0] == '+' || s[0] == '!') {
            EVAL_OPERATOR op_type;
            if (s[0] == '-') op_type = EVAL_OPERATOR_SIGN_MINUS;
            else if (s[0] == '+') op_type = EVAL_OPERATOR_SIGN_PLUS;
            else op_type = EVAL_OPERATOR_NOT;
            
            // Move past the unary operator
            s++;
            skip_spaces(&s);
            
            // Check if followed by a function
            if (starts_with_function(s)) {
                // Update the string position to include the consumed unary operator
                *string = s;
                
                // Parse the function
                EVAL_NODE *func_node = parse_function_call(string, error);
                if (!func_node) return NULL;
                
                // Create the unary operator node
                EVAL_NODE *op = eval_node_alloc(1);
                op->operator = op_type;
                op->precedence = eval_precedence(op_type);
                eval_node_set_value_to_node(op, 0, func_node);
                
                return op;
            }
        }
        // Check for a direct function call
        else if (starts_with_function(s)) {
            *string = s;
            return parse_function_call(string, error);
        }
    }
    
    // If no special handling needed, fall back to normal parsing
    return parse_full_expression(string, error);
}

// high level function to parse an expression or a sub-expression
static inline EVAL_NODE *parse_full_expression(const char **string, EVAL_ERROR *error) {
    EVAL_NODE *op1 = parse_one_full_operand(string, error);
    if(!op1) {
        *error = EVAL_ERROR_MISSING_OPERAND;
        return NULL;
    }

    return parse_rest_of_expression(string, error, op1);
}

// ----------------------------------------------------------------------------
// public API for parsing

EVAL_EXPRESSION *expression_parse(const char *string, const char **failed_at, EVAL_ERROR *error) {
    if(!string || !*string)
        return NULL;

    const char *s = string;
    EVAL_ERROR err = EVAL_ERROR_OK;
    EVAL_NODE *op = NULL;

#ifdef USE_RE2C_LEMON_PARSER
    // Use the re2c/lemon parser
    op = parse_expression_with_re2c_lemon(string, &s, &err);
#else
    // Use the original recursive descent parser
    
    // First, let's check if the expression starts with a function
    skip_spaces(&s);
    
    // Check if the expression starts with a unary op followed by function
    // Removed condition that was too restrictive - we want to handle any unary operator
    // followed by a function, not just when there's whitespace after the operator
    if (s[0] == '-' || s[0] == '+' || s[0] == '!') {
        
        const char *after_op = s + 1;
        skip_spaces(&after_op);
        
        if (starts_with_function(after_op)) {
            // This is a special case like "-abs(...)" - use our special parser
            s = string; // Reset to beginning
            op = parse_expression(&s, &err, 1);
        }
    }
    
    // If we haven't parsed it with the special function, use regular parsing
    if (!op) {
        s = string; // Reset to beginning
        op = parse_full_expression(&s, &err);
    }
#endif

    if(s && *s) {
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
    exp->local_variables = NULL; // Initialize local variables list

    return exp;
}
