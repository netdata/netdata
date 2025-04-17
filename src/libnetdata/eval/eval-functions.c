// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// Global registry for dynamic functions
EVAL_DYNAMIC_FUNCTION *eval_function_registry = NULL;

// Next available operator ID for dynamic functions (starting after hardcoded operators)
EVAL_OPERATOR next_function_op = EVAL_OPERATOR_CUSTOM_FUNCTION_START; // Start from a safe value

void str2lower(char *dst, const char *src) {
    while (*src) {
        *dst = (char)tolower((uint8_t)*src);
        dst++;
        src++;
    }
    *dst = '\0';
}

// Register a new function in the registry
bool eval_register_function(const char *name, eval_function_cb callback, int min_params, int max_params) {
    if (!name || !callback || min_params < 0)
        return 0;
    
    // make it lowercase
    char n[strlen(name) + 1];
    str2lower(n, name);
    name = n;

    // Check if a function with this name already exists
    EVAL_DYNAMIC_FUNCTION *func = eval_function_lookup(name);
    if (!func) {
        func = callocz(1, sizeof(EVAL_DYNAMIC_FUNCTION));
        func->name = string_strdupz(name);

        // Assign a unique operator ID
        func->operator = __atomic_fetch_add(&next_function_op, 1, __ATOMIC_RELAXED);

        // Add to registry
        func->next = eval_function_registry;
        eval_function_registry = func;
    }
    
    // Create new function registry entry
    func->callback = callback;
    func->min_params = min_params;
    func->max_params = max_params;

    // Register the function operator in the operators' table
    operators[func->operator].print_as = string2str(func->name);
    operators[func->operator].precedence = eval_precedence(EVAL_OPERATOR_FUNCTION);
    operators[func->operator].parameters = 0; // Will be set dynamically when the function is called
    operators[func->operator].isfunction = 1;
    operators[func->operator].eval = eval_execute_function;
    
    return 1;
}

// Lookup a function by name (case-insensitive)
EVAL_DYNAMIC_FUNCTION *eval_function_lookup(const char *name) {
    if (!name)
        return NULL;

    // make it lowercase
    char n[strlen(name) + 1];
    str2lower(n, name);
    name = n;

    // Create a temporary STRING for case-insensitive comparison
    STRING *lookup_name = string_strdupz(name);
    
    EVAL_DYNAMIC_FUNCTION *func = eval_function_registry;
    while (func) {
        // Compare names case-insensitively
        if (func->name == lookup_name) {
            string_freez(lookup_name);
            return func;
        }
        func = func->next;
    }

    string_freez(lookup_name);
    return NULL;
}

// Function to evaluate a dynamic function
NETDATA_DOUBLE eval_execute_function(struct eval_expression *exp, EVAL_NODE *op, EVAL_ERROR *error) {
    if (!op) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return NAN;
    }
    
    // Find the function in the registry
    EVAL_DYNAMIC_FUNCTION *func = NULL;
    for (func = eval_function_registry; func; func = func->next) {
        if (func->operator == op->operator)
            break;
    }
    
    if (!func) {
        buffer_sprintf(exp->error_msg, "unknown function with operator %d", (int)op->operator);
        *error = EVAL_ERROR_UNKNOWN_OPERAND;
        return NAN;
    }
    
    // Check parameter count
    if (op->count < func->min_params) {
        buffer_sprintf(exp->error_msg, "function %s requires at least %d parameters, but %d provided", 
                      string2str(func->name), func->min_params, op->count);
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return NAN;
    }
    
    if (func->max_params >= 0 && op->count > func->max_params) {
        buffer_sprintf(exp->error_msg, "function %s accepts at most %d parameters, but %d provided", 
                      string2str(func->name), func->max_params, op->count);
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return NAN;
    }
    
    // Call the function
    return func->callback(exp, op->count, op->ops, error);
}

// Define abs function using the dynamic system
static NETDATA_DOUBLE abs_function(struct eval_expression *exp, int param_count, EVAL_VALUE *params, EVAL_ERROR *error) {
    if (param_count != 1) {
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return NAN;
    }
    
    // Evaluate the parameter
    NETDATA_DOUBLE n = eval_value(exp, &params[0], error);
    if (*error != EVAL_ERROR_OK)
        return NAN;
    
    return ABS(n);
}

// Function to initialize the eval subsystem
void eval_functions_init(void) {
    // Initialize the function registry
    eval_register_function("abs", abs_function, 1, 1);
}
