/**
 * re2c lexer for Netdata's expression evaluator
 *
 * This implementation uses re2c for lexical analysis and lemon for parsing.
 * It is fully integrated with Netdata's existing EVAL_NODE structure.
 */

#include "../eval-internal.h"
#include "parser_internal.h"

// Scanner functions implementation
void scanner_init(Scanner *s, const char *input) {
    if (!input) {
        // Handle NULL input safely
        s->cursor = "";
        s->marker = s->cursor;
        s->token = s->cursor;
        s->limit = s->cursor;
        s->line = 1;
        s->error = 1;  // Set error flag for NULL input
        s->in_assignment = 0; // Initialize assignment context flag
        return;
    }
    
    s->cursor = input;
    s->marker = s->cursor;
    s->token = s->cursor;
    s->limit = s->cursor + strlen(s->cursor);
    s->line = 1;
    s->error = 0;  // Initialize error flag
    s->in_assignment = 0; // Initialize assignment context flag
}

int scan(Scanner *s, YYSTYPE *lval) {
    const char *YYMARKER;
    const char *YYCURSOR = s->cursor;
    char variable_buffer[EVAL_MAX_VARIABLE_NAME_LENGTH + 1] = {0};
    
    // Skip whitespace
    while (1) {
        s->token = YYCURSOR;
        
/*!re2c
    re2c:define:YYCTYPE = char;
    re2c:yyfill:enable = 0;
    
    // Skip spaces and tabs (but treat newlines as semicolons for multi-line scripts)
    [ \t]+ { continue; }
    
    // Treat newlines as semicolons
    [\r\n]+ { 
        s->cursor = YYCURSOR; 
        s->in_assignment = 0; 
        return TOK_SEMICOLON; 
    }
    
    // Special numeric literals - more comprehensive handling for various capitalizations
    // Support NaN with all case variations 
    // Matching str2ndd behavior in inlined.h which accepts "nan" (any case) and also "null"
    [nN][aA][nN] | [nN][uU][lL][lL] {
        lval->dval = NAN;
        s->cursor = YYCURSOR;
        s->in_assignment = 0; // Reset assignment context
        return TOK_NUMBER;
    }
    
    // Support Infinity with all case variations
    // Matching str2ndd behavior in inlined.h which accepts "inf" in any case
    [iI][nN][fF]([iI][nN][iI][tT][yY])? {
        lval->dval = INFINITY;
        s->cursor = YYCURSOR;
        s->in_assignment = 0; // Reset assignment context
        return TOK_NUMBER;
    }
    
    // Numbers
    [0-9]+ | 
    [0-9]+"."[0-9]* |
    "."[0-9]+ |
    [0-9]+[eE][+-]?[0-9]+ |
    [0-9]+"."[0-9]*[eE][+-]?[0-9]+ |
    "."[0-9]+[eE][+-]?[0-9]+ {
        char *endptr;
        lval->dval = str2ndd(s->token, &endptr);
        s->cursor = YYCURSOR;
        s->in_assignment = 0; // Reset assignment context
        return TOK_NUMBER;
    }
    
    // Variables - can contain any characters that aren't operators or closing brackets
    // The original parser allows any character that passes !is_operator_first_symbol_or_space(s) && s != ')' && s != '}'
    // We also need to exclude comma for proper function parameter parsing
    "$"[^\000 \t\r\n&|!><=%+\-*/?()}{;,]+ {
        size_t len = YYCURSOR - s->token - 1; // -1 to skip the $
        if (len >= EVAL_MAX_VARIABLE_NAME_LENGTH) {
            len = EVAL_MAX_VARIABLE_NAME_LENGTH - 1;
        }
        memcpy(variable_buffer, s->token + 1, len);
        variable_buffer[len] = '\0';
        lval->strval = strdupz(variable_buffer);
        s->cursor = YYCURSOR;
        s->in_assignment = 1; // Mark that we just saw a variable, potential assignment context
        return TOK_VARIABLE;
    }
    
    // Empty variable with braces - treat as error
    "${}"       { s->cursor = YYCURSOR; s->error = 1; return 0; }
    
    // Variables with braces - can contain any character except } and \0
    "${" [^}\000]* "}" {
        // Calculate length, excluding the ${ prefix and the } suffix
        size_t len = YYCURSOR - s->token - 3; // -3 to skip ${ and }
        if (len >= EVAL_MAX_VARIABLE_NAME_LENGTH) {
            len = EVAL_MAX_VARIABLE_NAME_LENGTH - 1;
        }
        memcpy(variable_buffer, s->token + 2, len);
        variable_buffer[len] = '\0';
        lval->strval = strdupz(variable_buffer);
        s->cursor = YYCURSOR;
        s->in_assignment = 1; // Mark that we just saw a variable, potential assignment context
        return TOK_VARIABLE;
    }
    
    // Operators
    "+" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_PLUS; }
    "-" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_MINUS; }
    "*" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_MULTIPLY; }
    "/" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_DIVIDE; }
    "%" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_MODULO; }
    ";" { s->cursor = YYCURSOR; s->in_assignment = 0; return TOK_SEMICOLON; }
    
    // Logical operators - full case-insensitive handling for AND, OR, NOT
    // Including versions with parentheses that push back the parenthesis
    "&&" | [aA][nN][dD][ \t]* {
        s->cursor = YYCURSOR; 
        return TOK_AND; 
    }
    
    [aA][nN][dD]"(" { 
        // Move cursor back to just before the '('
        s->cursor = YYCURSOR - 1; 
        return TOK_AND; 
    }
    
    "||" | [oO][rR][ \t]* {
        s->cursor = YYCURSOR; 
        return TOK_OR; 
    }
    
    [oO][rR]"(" { 
        // Move cursor back to just before the '('
        s->cursor = YYCURSOR - 1; 
        return TOK_OR; 
    }
    
    "!" | [nN][oO][tT][ \t]* {
        s->cursor = YYCURSOR; 
        return TOK_NOT; 
    }
    
    [nN][oO][tT]"(" { 
        // Move cursor back to just before the '('
        s->cursor = YYCURSOR - 1; 
        return TOK_NOT; 
    }
    
    // Comparison operators and assignment
    "=="        { s->cursor = YYCURSOR; return TOK_EQ; }
    "="         { 
        s->cursor = YYCURSOR;
        // If we're after a variable, it's an assignment, otherwise it's equality
        int token = s->in_assignment ? TOK_ASSIGN : TOK_EQ;
        s->in_assignment = 0; // Reset assignment context
        return token;
    }
    "!=" | "<>" { s->cursor = YYCURSOR; return TOK_NE; }
    "<"         { s->cursor = YYCURSOR; return TOK_LT; }
    "<="        { s->cursor = YYCURSOR; return TOK_LE; }
    ">"         { s->cursor = YYCURSOR; return TOK_GT; }
    ">="        { s->cursor = YYCURSOR; return TOK_GE; }
    
    // Ternary operator
    "?"         { s->cursor = YYCURSOR; return TOK_QMARK; }
    ":"         { s->cursor = YYCURSOR; return TOK_COLON; }
    
    // Parentheses and comma
    "("         { s->cursor = YYCURSOR; return TOK_LPAREN; }
    ")"         { s->cursor = YYCURSOR; return TOK_RPAREN; }
    ","         { s->cursor = YYCURSOR; return TOK_COMMA; }
    
    // Function names - now supports any valid function name followed by "("
    [a-zA-Z_][a-zA-Z0-9_]*"(" {
        // Get the function name (excluding the trailing parenthesis)
        size_t len = YYCURSOR - s->token - 1; // -1 to exclude the "("
        char func_name[256] = {0};
        if(len >= sizeof(func_name) - 1) {
            len = sizeof(func_name) - 1;
        }
        memcpy(func_name, s->token, len);
        func_name[len] = '\0';
        
        // Check if this is a registered function
        EVAL_DYNAMIC_FUNCTION *func = eval_function_lookup(func_name);
        if(func) {
            // It's a registered function
            lval->op = func->operator;
            s->cursor = YYCURSOR;
            return TOK_FUNCTION;
        }
        
        // Not a registered function - report error
        s->cursor = YYCURSOR;
        s->error = 1;
        return 0;
    }
    
    // Empty variable placeholders - these should be errors
    "${"        { s->cursor = YYCURSOR; s->error = 1; return 0; }
    
    // End of input
    "\000"      { s->cursor = YYCURSOR; return 0; }
    
    // Any other character is an error - set error flag and return 0 to stop parsing
    .           { 
        s->cursor = YYCURSOR; 
        s->error = 1;  // Set error flag
        return 0;      // Return 0 to stop parsing
    }
*/
    }
}

// Function to parse an expression with re2c/lemon
EVAL_NODE *parse_expression_with_re2c_lemon(const char *string, const char **failed_at, EVAL_ERROR *error) {
    Scanner scanner;
    scanner_init(&scanner, string);

    if(failed_at)
        *failed_at = NULL;
    
    // Use ParseAlloc with mallocz instead of malloc - mallocz will handle allocation failures
    void *parser = ParseAlloc(mallocz);
    
    EVAL_NODE *result = NULL;
    
    YYSTYPE token_value;
    int token_type;
    
    // Initialize error code
    if (error) *error = EVAL_ERROR_OK;
    
    // Save the token start position for error reporting
    const char *error_pos = scanner.cursor;
    
    // Variable to track if we need to free token_value.strval
    int free_strval = 0;
    
    while ((token_type = scan(&scanner, &token_value)) > 0) {
        // If the token is a variable, remember to free it if there's an error
        free_strval = (token_type == TOK_VARIABLE);
        
        Parse(parser, token_type, token_value, &result);
        
        // Save position before potential error
        error_pos = scanner.token;
        
        // Check for syntax errors after each token
        if (result && result->operator == EVAL_OPERATOR_NOP && result->count == 0) {
            // This is an error marker
            if (error) *error = EVAL_ERROR_SYNTAX;
            if (failed_at) {
                *failed_at = error_pos;
            }
            
            // Clean up
            eval_node_free(result);
            ParseFree(parser, freez);
            
            // If we just scanned a variable, free its strval
            if (free_strval && token_value.strval) {
                freez(token_value.strval);
            }
            
            return NULL;
        }
        
        // Reset free_strval since the parser has taken ownership of the string
        free_strval = 0;
    }
    
    // If the last token was a variable and scanning stopped due to an error,
    // we need to free the token_value.strval
    if (free_strval && token_value.strval) {
        freez(token_value.strval);
        token_value.strval = NULL;
    }
    
    // Finish parsing
    Parse(parser, 0, token_value, &result);
    
    // Clean up the parser
    ParseFree(parser, freez);
    
    // Check for lexer errors
    if (scanner.error) {
        if (error) *error = EVAL_ERROR_UNKNOWN_OPERAND;
        if (failed_at) {
            *failed_at = error_pos;
        }
        
        // Clean up result if it was created
        if (result) {
            eval_node_free(result);
        }
        
        return NULL;
    }
    
    if (!result) {
        if (error) *error = EVAL_ERROR_SYNTAX;
        if (failed_at) {
            *failed_at = error_pos;
        }
        return NULL;
    }
    
    if (failed_at)
        *failed_at = NULL;
    
    return result;
}