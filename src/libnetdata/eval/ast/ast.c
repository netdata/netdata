#include "libnetdata/libnetdata.h"
#include "ast.h"

ASTNode *eval_ast_parse_string(const char *input);
ASTNode *parse_expression_ast(const char *expression) {
    ASTNode *result = eval_ast_parse_string(expression);

    if(!result) {
        // TODO: cleanup incomplete allocated nodes
        return NULL;
    }

    return result;
}

ASTNode **create_ast_nodes_array(ASTNode **nodes, int count) {
    ASTNode **node_array = reallocz(nodes, count * sizeof(ASTNode *));
    return node_array;
}

ASTNode *create_ast_node(void) {
    ASTNode *node = callocz(1, sizeof(ASTNode));
    return node;
}

/* Create a literal node */
ASTNode *create_literal_node(double value)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_LITERAL;
    node->data.literal = value;
    return node;
}

/* Create a variable node */
ASTNode *create_variable_node(char *name)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_VARIABLE;
    // Store a copy of the name - the original will be freed by the lexer
    node->data.variable = name;
    return node;
}

/* Create a binary operation node */
ASTNode *create_binary_op_node(BinaryOp op, ASTNode *left, ASTNode *right)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_BINARY_OP;
    node->data.binary_op.op = op;
    node->data.binary_op.left = left;
    node->data.binary_op.right = right;
    return node;
}

/* Create a unary operation node */
ASTNode *create_unary_op_node(UnaryOp op, ASTNode *operand)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_UNARY_OP;
    node->data.unary_op.op = op;
    node->data.unary_op.operand = operand;
    return node;
}

/* Create a function call node */
ASTNode *create_function_call_node(char *name, ArgList args)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_FUNCTION_CALL;
    // Store a copy of the name - the original will be freed by the lexer
    node->data.function_call.name = name;
    node->data.function_call.args = args;
    return node;
}

/* Create a ternary operation node */
ASTNode *create_ternary_op_node(ASTNode *condition, ASTNode *true_expr, ASTNode *false_expr)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_TERNARY_OP;
    node->data.ternary_op.condition = condition;
    node->data.ternary_op.true_expr = true_expr;
    node->data.ternary_op.false_expr = false_expr;
    return node;
}

/* Create an assignment node */
ASTNode *create_assignment_node(char *variable, ASTNode *value)
{
    ASTNode *node = create_ast_node();
    node->type = NODE_ASSIGNMENT;
    // Store a copy of the variable name - the original will be freed by the lexer
    node->data.assignment.variable = variable;
    node->data.assignment.value = value;
    return node;
}

/* Free memory allocated for AST */
void eval_ast_node_free(ASTNode *node)
{
    if (!node)
        return;

    switch (node->type) {
        case NODE_LITERAL:
            break;

        case NODE_VARIABLE:
            freez(node->data.variable);
            break;

        case NODE_BINARY_OP:
            eval_ast_node_free(node->data.binary_op.left);
            eval_ast_node_free(node->data.binary_op.right);
            break;

        case NODE_UNARY_OP:
            eval_ast_node_free(node->data.unary_op.operand);
            break;

        case NODE_FUNCTION_CALL:
            freez(node->data.function_call.name);
            for (int i = 0; i < node->data.function_call.args.count; i++) {
                eval_ast_node_free(node->data.function_call.args.args[i]);
            }
            freez(node->data.function_call.args.args);
            break;

        case NODE_TERNARY_OP:
            eval_ast_node_free(node->data.ternary_op.condition);
            eval_ast_node_free(node->data.ternary_op.true_expr);
            eval_ast_node_free(node->data.ternary_op.false_expr);
            break;

        case NODE_ASSIGNMENT:
            freez(node->data.assignment.variable);
            eval_ast_node_free(node->data.assignment.value);
            break;
    }

    freez(node);
}

/* Helper function to get operator string */
const char *get_binary_op_str(BinaryOp op)
{
    switch (op) {
        case OP_ADD:
            return "+";
        case OP_SUB:
            return "-";
        case OP_MUL:
            return "*";
        case OP_DIV:
            return "/";
        case OP_MOD:
            return "%";
        case OP_POW:
            return "^";
        case OP_EQ:
            return "==";
        case OP_NE:
            return "!=";
        case OP_LT:
            return "<";
        case OP_LE:
            return "<=";
        case OP_GT:
            return ">";
        case OP_GE:
            return ">=";
        case OP_AND:
            return "&&";
        case OP_OR:
            return "||";
        default:
            return "unknown";
    }
}

const char *get_unary_op_str(UnaryOp op)
{
    switch (op) {
        case OP_NEG:
            return "-";
        case OP_NOT:
            return "!";
        default:
            return "unknown";
    }
}

/* Print the AST for debugging */
void print_ast(ASTNode *node, int indent)
{
    if (!node)
        return;

    char spaces[100];
    for (int i = 0; i < indent; i++)
        spaces[i] = ' ';
    spaces[indent] = '\0';

    switch (node->type) {
        case NODE_LITERAL:
            printf("%sLITERAL: %g\n", spaces, node->data.literal);
            break;

        case NODE_VARIABLE:
            printf("%sVARIABLE: %s\n", spaces, node->data.variable);
            break;

        case NODE_BINARY_OP:
            printf("%sBINARY_OP: %s\n", spaces, get_binary_op_str(node->data.binary_op.op));
            print_ast(node->data.binary_op.left, indent + 2);
            print_ast(node->data.binary_op.right, indent + 2);
            break;

        case NODE_UNARY_OP:
            printf("%sUNARY_OP: %s\n", spaces, get_unary_op_str(node->data.unary_op.op));
            print_ast(node->data.unary_op.operand, indent + 2);
            break;

        case NODE_FUNCTION_CALL:
            printf("%sFUNCTION_CALL: %s\n", spaces, node->data.function_call.name);
            for (int i = 0; i < node->data.function_call.args.count; i++) {
                printf("%s  ARG %d:\n", spaces, i + 1);
                print_ast(node->data.function_call.args.args[i], indent + 4);
            }
            break;

        case NODE_TERNARY_OP:
            printf("%sTERNARY_OP:\n", spaces);
            printf("%s  CONDITION:\n", spaces);
            print_ast(node->data.ternary_op.condition, indent + 4);
            printf("%s  TRUE_EXPR:\n", spaces);
            print_ast(node->data.ternary_op.true_expr, indent + 4);
            printf("%s  FALSE_EXPR:\n", spaces);
            print_ast(node->data.ternary_op.false_expr, indent + 4);
            break;

        case NODE_ASSIGNMENT:
            printf("%sASSIGNMENT: %s\n", spaces, node->data.assignment.variable);
            print_ast(node->data.assignment.value, indent + 2);
            break;
    }
}
