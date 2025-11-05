#ifndef EVAL_RE2C_LEMON_INTERNAL_H
#define EVAL_RE2C_LEMON_INTERNAL_H

#include "../eval-internal.h"
#include "parser.h" // This has the token definitions (TOK_*)

// Token values for re2c lexer
typedef union {
    NETDATA_DOUBLE dval;
    char *strval;
    EVAL_NODE *node;  // Added for handling node pointers
    EVAL_OPERATOR op; // Operator ID for dynamic functions
} YYSTYPE;

// Scanner structure definition
typedef struct {
    const char *cursor;
    const char *marker;
    const char *token;
    const char *limit;
    int line;
    int error;  // Flag to indicate a lexer error occurred
    int in_assignment; // Flag to indicate if we are in an assignment context
} Scanner;

// Function declarations for the scanner
void scanner_init(Scanner *s, const char *input);
int scan(Scanner *s, YYSTYPE *lval);

// Function declarations for the parser
void *ParseAlloc(void *(*mallocProc)(size_t));
void ParseFree(void *p, void (*freeProc)(void*));
void Parse(void *yyp, int yymajor, YYSTYPE yyminor, EVAL_NODE **result);

// Additional error code
#define EVAL_ERROR_SYNTAX EVAL_ERROR_UNKNOWN_OPERAND

#endif // EVAL_RE2C_LEMON_INTERNAL_H