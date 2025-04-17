// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// Function to get a local variable's value
NETDATA_DOUBLE get_local_variable_value(EVAL_EXPRESSION *exp, STRING *var_name, EVAL_ERROR *error) {
    *error = EVAL_ERROR_OK;
    
    // Check if the variable exists in the local variable list
    EVAL_LOCAL_VARIABLE *var = exp->local_variables;
    while (var) {
        if (var->name == var_name) {
            return var->value;
        }
        var = var->next;
    }
    
    // If not found, try to get it from the global callback
    NETDATA_DOUBLE value;
    if (exp->variable_lookup_cb && exp->variable_lookup_cb(var_name, exp->variable_lookup_cb_data, &value)) {
        return value;
    }
    
    // Variable not found
    *error = EVAL_ERROR_UNKNOWN_VARIABLE;
    return NAN;
}

// Function to set a local variable's value
void set_local_variable_value(EVAL_EXPRESSION *exp, STRING *var_name, NETDATA_DOUBLE value) {
    // Check if the variable already exists
    EVAL_LOCAL_VARIABLE *var = exp->local_variables;
    while (var) {
        if (var->name == var_name) {
            // Update existing variable
            var->value = value;
            return;
        }
        var = var->next;
    }
    
    // Variable doesn't exist, create a new one
    var = mallocz(sizeof(EVAL_LOCAL_VARIABLE));
    var->name = string_dup(var_name);
    var->value = value;
    var->next = exp->local_variables;
    exp->local_variables = var;
}