#include <stdio.h>

#include "ast.h"
#include "parser.tab.h"

extern ASTNode *parse_string(const char *input);

int main(int argc, char *argv[])
{
    (void)argc;
    (void)argv;

    /* Expressions to parse - extracted from the test suite */
    const char *expressions[] = {// Arithmetic operations
                                 "1 + 2",
                                 "5 - 3",
                                 "4 * 5",
                                 "10 / 2",
                                 "-10",
                                 "+5",
                                 "1 + 2 * 3",
                                 "(1 + 2) * 3",
                                 "10.5 + 2.5",
                                 "1.5e2 + 2",

                                 // Comparison operations
                                 "1 == 1",
                                 "1 != 2",
                                 "5 > 3",
                                 "3 < 5",
                                 "5 >= 5",
                                 "5 <= 4",

                                 // Logical operations
                                 "1 && 1",
                                 "1 || 0",
                                 "!1",
                                 "!(1 && 0)",
                                 "0 || !(1 && 0)",

                                 // Variables
                                 "$var1",
                                 "$var2",
                                 "$var1 + $var2",
                                 "$var1 * $var2",
                                 "${var1}",
                                 "${this variable}",
                                 "${this} + ${this variable}",

                                 // Functions
                                 "abs(5)",
                                 "abs(-5)",
                                 "abs($var1)",
                                 "abs($negative)",
                                 "abs(abs(-5))",
                                 "abs(-($var1 - $var2))",

                                 // Ternary operator
                                 "(1 > 0) ? 10 : 20",
                                 "(0 > 1) ? 10 : 20",
                                 "($var1 > $var2) ? ($var1 - $var2) : ($var2 - $var1)",
                                 "($var1 > 0) ? (($var1 < 0) ? 1 : 2) : 3",

                                 // Complex expressions
                                 "1 + 2 * 3 - 4 / 2",
                                 "(1 + 2) * (3 - 4) / 2",
                                 "5 > 3 && 2 < 4 || 1 == 0",
                                 "((($var1 + $var2) / 2) > 30) ? ($var1 * $var2) : ($var1 + $var2)",
                                 "($var1 > 40 && $var2 < 30) || ($var1 - $var2 > 10)",
                                 "(5 + 3 * 2) / (1 + 1) * 4 - 10",
                                 "((((($var1 / 2) + ($var2 * 2)) - 10) * 2) / 4) + (($var1 > $var2) ? 5 : -5)",
                                 "(($zero)) ? 0 : ((($var1)))",
                                 "!($var1 < 40) && ($var2 > 20 || $zero < 1) && !($var1 == $var2)",

                                 // Scientific notation and special values
                                 "1e308",
                                 "-1e308",
                                 "$nan_var == $nan_var",
                                 "$inf_var > 5",
                                 "$zero && (1 / $zero)",
                                 "1 || (1e308 * 1e308)"};

    int count = sizeof(expressions) / sizeof(expressions[0]);

    for (int i = 0; i < count; i++) {
        printf("\n[%d] Parsing: %s\n", i, expressions[i]);

        /* Parse the expression */
        ASTNode *ast = parse_string(expressions[i]);

        if (ast != NULL) {
            /* Print the AST */
            printf("AST Structure:\n");
            print_ast(ast, 2);

            /* Here you would process the AST as needed */

            /* Free the AST when done */
            free_ast(ast);
        } else {
            printf("Failed to parse expression\n");
        }
    }

    return 0;
}
