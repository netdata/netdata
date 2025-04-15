%{
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "ast.h"

extern int yylex();
extern char* yytext;
extern FILE* yyin;

void yyerror(const char *s);
ASTNode* ast_root = NULL;
%}

%union {
    double dval;
    char* sval;
    ASTNode* node;
    ArgList arg_list;
}

%token <dval> NUMBER
%token <sval> VARIABLE FUNCTION
%token PLUS MINUS MULTIPLY DIVIDE MODULO POWER
%token EQ NE LT LE GT GE
%token AND OR NOT
%token QMARK COLON
%token LPAREN RPAREN COMMA
%token ASSIGN

%type <node> expr program
%type <arg_list> arg_list

/* Operator precedence - lowest to highest */
%right ASSIGN
%left OR
%left AND
%left EQ NE
%left LT LE GT GE
%left PLUS MINUS
%left MULTIPLY DIVIDE MODULO
%right POWER
%right NOT UMINUS UPLUS
%right QMARK COLON

%start program

%%

program
    : expr                      { 
                                  ast_root = $1; 
                                  printf("AST built successfully\n");
                                  $$ = $1;
                                }
    | VARIABLE ASSIGN expr      { 
                                  ast_root = create_assignment_node($1, $3);
                                  printf("Assignment AST built successfully\n");
                                  $$ = ast_root;
                                }
    ;

expr
    : expr PLUS expr            { $$ = create_binary_op_node(OP_ADD, $1, $3); }
    | expr MINUS expr           { $$ = create_binary_op_node(OP_SUB, $1, $3); }
    | expr MULTIPLY expr        { $$ = create_binary_op_node(OP_MUL, $1, $3); }
    | expr DIVIDE expr          { $$ = create_binary_op_node(OP_DIV, $1, $3); }
    | expr MODULO expr          { $$ = create_binary_op_node(OP_MOD, $1, $3); }
    | expr POWER expr           { $$ = create_binary_op_node(OP_POW, $1, $3); }
    | expr EQ expr              { $$ = create_binary_op_node(OP_EQ, $1, $3); }
    | expr NE expr              { $$ = create_binary_op_node(OP_NE, $1, $3); }
    | expr LT expr              { $$ = create_binary_op_node(OP_LT, $1, $3); }
    | expr LE expr              { $$ = create_binary_op_node(OP_LE, $1, $3); }
    | expr GT expr              { $$ = create_binary_op_node(OP_GT, $1, $3); }
    | expr GE expr              { $$ = create_binary_op_node(OP_GE, $1, $3); }
    | expr AND expr             { $$ = create_binary_op_node(OP_AND, $1, $3); }
    | expr OR expr              { $$ = create_binary_op_node(OP_OR, $1, $3); }
    | NOT expr                  { $$ = create_unary_op_node(OP_NOT, $2); }
    | MINUS expr %prec UMINUS   { $$ = create_unary_op_node(OP_NEG, $2); }
    | PLUS expr %prec UPLUS     { $$ = $2; }
    | LPAREN expr RPAREN        { $$ = $2; }
    | NUMBER                    { $$ = create_literal_node($1); }
    | VARIABLE                  { $$ = create_variable_node($1); }
    | FUNCTION LPAREN arg_list RPAREN { 
                                  $$ = create_function_call_node($1, $3); 
                                }
    | expr QMARK expr COLON expr { $$ = create_ternary_op_node($1, $3, $5); }
    ;

arg_list
    : /* empty */               { $$.args = NULL; $$.count = 0; }
    | expr                      { 
                                  $$.args = malloc(sizeof(ASTNode*)); 
                                  $$.args[0] = $1; 
                                  $$.count = 1; 
                                }
    | arg_list COMMA expr       { 
                                  $$.count = $1.count + 1;
                                  $$.args = realloc($1.args, $$.count * sizeof(ASTNode*));
                                  $$.args[$$.count - 1] = $3;
                                }
    ;

%%

void yyerror(const char *s) {
    fprintf(stderr, "Error: %s\n", s);
}

/*
int main(int argc, char **argv) {
    if (argc > 1) {
        yyin = fopen(argv[1], "r");
        if (!yyin) {
            fprintf(stderr, "Cannot open file %s\n", argv[1]);
            return 1;
        }
    }
    
    int result = yyparse();
    
    if (result == 0 && ast_root != NULL) {
        printf("Printing AST:\n");
        print_ast(ast_root, 0);
        free_ast(ast_root);
    }
    
    return result;
}
*/

typedef struct yy_buffer_state * YY_BUFFER_STATE;
extern YY_BUFFER_STATE yy_scan_string(const char * str); // it does not work.
extern YY_BUFFER_STATE yy_scan_buffer(char *, size_t);
extern void yy_delete_buffer(YY_BUFFER_STATE buffer);

/* Parse a string and return the AST */
ASTNode* parse_string(const char* input) {
    void* buffer;
    ASTNode* result = NULL;
    
    /* Reset global ast_root */
    ast_root = NULL;
    
    /* Set up lexer to read from string */
    buffer = yy_scan_string(input);
    
    /* Parse the input */
    if (yyparse() == 0 && ast_root != NULL) {
        result = ast_root;
    }
    
    /* Clean up */
    yy_delete_buffer(buffer);
    
    return result;
}

