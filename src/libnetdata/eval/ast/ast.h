#ifndef AST_H
#define AST_H

/* AST node types */
typedef enum {
    NODE_LITERAL,       // Numeric literal
    NODE_VARIABLE,      // Variable reference
    NODE_BINARY_OP,     // Binary operation
    NODE_UNARY_OP,      // Unary operation
    NODE_FUNCTION_CALL, // Function call
    NODE_TERNARY_OP,    // Ternary operation
    NODE_ASSIGNMENT     // Assignment operation
} NodeType;

/* Binary operators */
typedef enum {
    OP_ADD,
    OP_SUB,
    OP_MUL,
    OP_DIV,
    OP_MOD,
    OP_POW,
    OP_EQ,
    OP_NE,
    OP_LT,
    OP_LE,
    OP_GT,
    OP_GE,
    OP_AND,
    OP_OR
} BinaryOp;

/* Unary operators */
typedef enum { OP_NEG, OP_NOT } UnaryOp;

/* Forward declaration for AST node */
typedef struct ASTNode ASTNode;

/* Function call arguments */
typedef struct {
    ASTNode **args;
    int count;
} ArgList;

/* AST node structure */
struct ASTNode {
    NodeType type;
    union {
        double literal; // For NODE_LITERAL
        char *variable; // For NODE_VARIABLE

        struct { // For NODE_BINARY_OP
            BinaryOp op;
            ASTNode *left;
            ASTNode *right;
        } binary_op;

        struct { // For NODE_UNARY_OP
            UnaryOp op;
            ASTNode *operand;
        } unary_op;

        struct { // For NODE_FUNCTION_CALL
            char *name;
            ArgList args;
        } function_call;

        struct { // For NODE_TERNARY_OP
            ASTNode *condition;
            ASTNode *true_expr;
            ASTNode *false_expr;
        } ternary_op;

        struct { // For NODE_ASSIGNMENT
            char *variable;
            ASTNode *value;
        } assignment;
    } data;
};

/* Function prototypes */
ASTNode *create_literal_node(double value);
ASTNode *create_variable_node(char *name);
ASTNode *create_binary_op_node(BinaryOp op, ASTNode *left, ASTNode *right);
ASTNode *create_unary_op_node(UnaryOp op, ASTNode *operand);
ASTNode *create_function_call_node(char *name, ArgList args);
ASTNode *create_ternary_op_node(ASTNode *condition, ASTNode *true_expr, ASTNode *false_expr);
ASTNode *create_assignment_node(char *variable, ASTNode *value);
void free_ast(ASTNode *node);
void print_ast(ASTNode *node, int indent);

#endif /* AST_H */
