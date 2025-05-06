%include {
#include "../eval-internal.h"
#include "parser_internal.h"
#include <assert.h>
}

%token_type {YYSTYPE}
%token_prefix TOK_

%type expr {EVAL_NODE*}
%type program {EVAL_NODE*}

%syntax_error {
    // Create a NOP node with count=0 as an error marker
    EVAL_NODE *error_node = eval_node_alloc(0);
    error_node->operator = EVAL_OPERATOR_NOP;
    *result = error_node;
}

%parse_accept {
    // Successfully parsed the expression
}

%parse_failure {
    // Failed to parse the expression
    if (*result) {
        eval_node_free(*result);
        *result = NULL;
    }
}

%extra_argument {EVAL_NODE **result}

%destructor expr {
    if ($$) {
        eval_node_free($$);
    }
}

// Start symbol
program ::= expr(E). {
    *result = E;
}

// Basic expressions
expr(A) ::= NUMBER(B). {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_NOP;
    eval_node_set_value_to_constant(A, 0, B.dval);
}

expr(A) ::= VARIABLE(B). {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_NOP;
    eval_node_set_value_to_variable(A, 0, B.strval);
    freez(B.strval); // Free the strdup'd string
}

// Parenthesized expressions
expr(A) ::= LPAREN expr(B) RPAREN. {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_EXPRESSION_OPEN;
    A->precedence = eval_precedence(EVAL_OPERATOR_EXPRESSION_OPEN);
    eval_node_set_value_to_node(A, 0, B);
}

// Unary operators
expr(A) ::= PLUS expr(B). [UPLUS] {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_SIGN_PLUS;
    A->precedence = eval_precedence(EVAL_OPERATOR_SIGN_PLUS);
    eval_node_set_value_to_node(A, 0, B);
}

expr(A) ::= MINUS expr(B). [UMINUS] {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_SIGN_MINUS;
    A->precedence = eval_precedence(EVAL_OPERATOR_SIGN_MINUS);
    eval_node_set_value_to_node(A, 0, B);
}

expr(A) ::= NOT expr(B). {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_NOT;
    A->precedence = eval_precedence(EVAL_OPERATOR_NOT);
    eval_node_set_value_to_node(A, 0, B);
}

// Function calls
expr(A) ::= FUNCTION_ABS LPAREN expr(B) RPAREN. {
    A = eval_node_alloc(1);
    A->operator = EVAL_OPERATOR_ABS;
    A->precedence = eval_precedence(EVAL_OPERATOR_ABS);
    eval_node_set_value_to_node(A, 0, B);
}

// Binary operators
expr(A) ::= expr(B) PLUS expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_PLUS;
    A->precedence = eval_precedence(EVAL_OPERATOR_PLUS);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) MINUS expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_MINUS;
    A->precedence = eval_precedence(EVAL_OPERATOR_MINUS);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) MULTIPLY expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_MULTIPLY;
    A->precedence = eval_precedence(EVAL_OPERATOR_MULTIPLY);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) DIVIDE expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_DIVIDE;
    A->precedence = eval_precedence(EVAL_OPERATOR_DIVIDE);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) MODULO expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_MODULO;
    A->precedence = eval_precedence(EVAL_OPERATOR_MODULO);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) AND expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_AND;
    A->precedence = eval_precedence(EVAL_OPERATOR_AND);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) OR expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_OR;
    A->precedence = eval_precedence(EVAL_OPERATOR_OR);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) EQ expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_EQUAL;
    A->precedence = eval_precedence(EVAL_OPERATOR_EQUAL);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) NE expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_NOT_EQUAL;
    A->precedence = eval_precedence(EVAL_OPERATOR_NOT_EQUAL);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) LT expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_LESS;
    A->precedence = eval_precedence(EVAL_OPERATOR_LESS);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) LE expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_LESS_THAN_OR_EQUAL;
    A->precedence = eval_precedence(EVAL_OPERATOR_LESS_THAN_OR_EQUAL);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) GT expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_GREATER;
    A->precedence = eval_precedence(EVAL_OPERATOR_GREATER);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

expr(A) ::= expr(B) GE expr(C). {
    A = eval_node_alloc(2);
    A->operator = EVAL_OPERATOR_GREATER_THAN_OR_EQUAL;
    A->precedence = eval_precedence(EVAL_OPERATOR_GREATER_THAN_OR_EQUAL);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
}

// Ternary operator with proper precedence and associativity
// This rule should ensure that ternary operators are right-associative
// and have lower precedence than comparison operators
expr(A) ::= expr(B) QMARK expr(C) COLON expr(D). {
    A = eval_node_alloc(3);
    A->operator = EVAL_OPERATOR_IF_THEN_ELSE;
    A->precedence = eval_precedence(EVAL_OPERATOR_IF_THEN_ELSE);
    eval_node_set_value_to_node(A, 0, B);
    eval_node_set_value_to_node(A, 1, C);
    eval_node_set_value_to_node(A, 2, D);
}

// Operator precedence declarations - LOWEST to HIGHEST
// In Lemon (like yacc/bison), precedence increases as you go down the list
//
// This means:
// 1. Ternary operator (?:) has the lowest precedence (will be evaluated last)
// 2. Logical operators (AND, OR) have the next lowest precedence
// 3. Comparison operators (EQ, NE, LT, etc.) are next
// 4. Addition and subtraction come next
// 5. Multiplication, division, and modulo have higher precedence
// 6. Unary operators (-, +, !) have the highest precedence (will be evaluated first)

// The %left and %right directives specify associativity:
// - %left: left-associative (a + b + c is parsed as (a + b) + c)
// - %right: right-associative (a = b = c is parsed as a = (b = c))

%right COLON QMARK.    // Ternary operator (right-associative)
%left OR AND.          // Logical operators
%left EQ NE.           // Equality operators
%left LT LE GT GE.     // Comparison operators
%left PLUS MINUS.      // Addition and subtraction
%left MULTIPLY DIVIDE MODULO.  // Multiplication, division, and modulo
%right UMINUS UPLUS NOT.       // Unary operators (highest precedence)