%{
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include "ast.h"
#include "parser.tab.h"

void yyerror(const char *s);
%}

%%
[0-9]+                          { yylval.dval = atof(yytext); return NUMBER; }
[0-9]+"."[0-9]*                 { yylval.dval = atof(yytext); return NUMBER; }
"."[0-9]+                       { yylval.dval = atof(yytext); return NUMBER; }
[0-9]+[eE][+-]?[0-9]+           { yylval.dval = atof(yytext); return NUMBER; }
[0-9]+"."[0-9]*[eE][+-]?[0-9]+  { yylval.dval = atof(yytext); return NUMBER; }
"."[0-9]+[eE][+-]?[0-9]+        { yylval.dval = atof(yytext); return NUMBER; }

\$[a-zA-Z][a-zA-Z0-9_]*         { yylval.sval = strdup(yytext); return VARIABLE; }
\$\{[a-zA-Z][a-zA-Z0-9_ _]*\}   { yylval.sval = strdup(yytext); return VARIABLE; }

"+"                             { return PLUS; }
"-"                             { return MINUS; }
"*"                             { return MULTIPLY; }
"/"                             { return DIVIDE; }
"%"                             { return MODULO; }
"^"                             { return POWER; }

"=="                            { return EQ; }
"!="                            { return NE; }
"<"                             { return LT; }
"<="                            { return LE; }
">"                             { return GT; }
">="                            { return GE; }

"&&"                            { return AND; }
"||"                            { return OR; }
"!"                             { return NOT; }

"?"                             { return QMARK; }
":"                             { return COLON; }

"("                             { return LPAREN; }
")"                             { return RPAREN; }
","                             { return COMMA; }
"="                             { return ASSIGN; }

[a-zA-Z][a-zA-Z0-9]*            { yylval.sval = strdup(yytext); return FUNCTION; }

[ \t\n\r]                       { /* Skip whitespace */ }
.                               { yyerror("Invalid character"); }
%%

int yywrap(void) {
    return 1;
}
