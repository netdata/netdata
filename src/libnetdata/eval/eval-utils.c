// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"
#include "eval-internal.h"

// ----------------------------------------------------------------------------
// memory management

EVAL_NODE *eval_node_alloc(int count) {
    static int id = 1;

    EVAL_NODE *op = callocz(1, sizeof(EVAL_NODE) + (sizeof(EVAL_VALUE) * count));

    op->id = id++;
    op->operator = EVAL_OPERATOR_NOP;
    op->precedence = 0;  // Will be set based on the operator
    op->count = count;
    return op;
}

void eval_node_set_value_to_node(EVAL_NODE *op, int pos, EVAL_NODE *value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_EXPRESSION;
    op->ops[pos].expression = value;
}

void eval_node_set_value_to_constant(EVAL_NODE *op, int pos, NETDATA_DOUBLE value) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_NUMBER;
    op->ops[pos].number = value;
}

void eval_node_set_value_to_variable(EVAL_NODE *op, int pos, const char *variable) {
    if(pos >= op->count)
        fatal("Invalid request to set position %d of OPERAND that has only %d values", pos + 1, op->count + 1);

    op->ops[pos].type = EVAL_VALUE_VARIABLE;
    op->ops[pos].variable = callocz(1, sizeof(EVAL_VARIABLE));
    op->ops[pos].variable->name = string_strdupz(variable);
}

void eval_variable_free(EVAL_VARIABLE *v) {
    string_freez(v->name);
    freez(v);
}

void eval_value_free(EVAL_VALUE *v) {
    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            eval_node_free(v->expression);
            break;

        case EVAL_VALUE_VARIABLE:
            eval_variable_free(v->variable);
            break;

        default:
            break;
    }
}

void eval_node_free(EVAL_NODE *op) {
    if(!op) return;

    if(op->count) {
        int i;
        for(i = op->count - 1; i >= 0 ;i--)
            eval_value_free(&op->ops[i]);
    }

    freez(op);
}

// ----------------------------------------------------------------------------
// parsed-as generation

void print_parsed_as_variable(BUFFER *out, EVAL_VARIABLE *v, EVAL_ERROR *error __maybe_unused) {
    buffer_sprintf(out, "${%s}", string2str(v->name));
}

void print_parsed_as_constant(BUFFER *out, NETDATA_DOUBLE n) {
    if(unlikely(isnan(n))) {
        buffer_strcat(out, "nan");
        return;
    }

    if(unlikely(isinf(n))) {
        buffer_strcat(out, "inf");
        return;
    }

    char b[100+1], *s;
    snprintfz(b, sizeof(b) - 1, NETDATA_DOUBLE_FORMAT, n);

    s = &b[strlen(b) - 1];
    while(s > b && *s == '0') {
        *s ='\0';
        s--;
    }

    if(s > b && *s == '.')
        *s = '\0';

    buffer_strcat(out, b);
}

void print_parsed_as_value(BUFFER *out, EVAL_VALUE *v, EVAL_ERROR *error) {
    switch(v->type) {
        case EVAL_VALUE_EXPRESSION:
            print_parsed_as_node(out, v->expression, error);
            break;

        case EVAL_VALUE_NUMBER:
            print_parsed_as_constant(out, v->number);
            break;

        case EVAL_VALUE_VARIABLE:
            print_parsed_as_variable(out, v->variable, error);
            break;

        default:
            *error = EVAL_ERROR_INVALID_VALUE;
            break;
    }
}

void print_parsed_as_node(BUFFER *out, EVAL_NODE *op, EVAL_ERROR *error) {
    extern struct operator operators[];

    if(unlikely(!op)) {
        buffer_strcat(out, "NULL");
        *error = EVAL_ERROR_INVALID_VALUE;
        return;
    }

    if(unlikely(!has_the_right_number_of_operands(op))) {
        buffer_sprintf(out, "INVALID PARAMETERS (operator requires %d, but node has %d)", operators[op->operator].parameters, op->count);
        *error = EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS;
        return;
    }

    if(operators[op->operator].isfunction) {
        if(op->operator < EVAL_OPERATOR_CUSTOM_FUNCTION_START || !operators[op->operator].print_as) {
            buffer_sprintf(out, "INVALID FUNCTION (operator %d)", op->operator);
            *error = EVAL_ERROR_INVALID_OPERAND;
            return;
        }

        buffer_strcat(out, operators[op->operator].print_as);
        buffer_strcat(out, "(");
        
        // Print function parameters
        // Dynamic functions have variable parameter counts
        int param_count = op->count;
        
        for(int i = 0; i < param_count; i++) {
            if(i > 0) buffer_strcat(out, ", ");
            print_parsed_as_value(out, &op->ops[i], error);
        }

        buffer_strcat(out, ")");
        return;
    }

    if(op->operator == EVAL_OPERATOR_NOP) {
        print_parsed_as_value(out, &op->ops[0], error);
        return;
    }
    
    if(op->operator == EVAL_OPERATOR_IF_THEN_ELSE) {
        print_parsed_as_value(out, &op->ops[0], error);
        buffer_strcat(out, " ? ");
        print_parsed_as_value(out, &op->ops[1], error);
        buffer_strcat(out, " : ");
        print_parsed_as_value(out, &op->ops[2], error);
        return;
    }

    if(op->count == 1) {
        buffer_strcat(out, operators[op->operator].print_as);
        buffer_strcat(out, "(");
        print_parsed_as_value(out, &op->ops[0], error);
        buffer_strcat(out, ")");
        return;
    }

    buffer_strcat(out, "(");
    print_parsed_as_value(out, &op->ops[0], error);
    buffer_strcat(out, " ");
    buffer_strcat(out, operators[op->operator].print_as);
    buffer_strcat(out, " ");
    print_parsed_as_value(out, &op->ops[1], error);
    buffer_strcat(out, ")");
}

// ----------------------------------------------------------------------------
// public API utility functions

const char *expression_strerror(EVAL_ERROR error) {
    switch(error) {
        case EVAL_ERROR_OK:
            return "success";

        case EVAL_ERROR_MISSING_CLOSE_SUBEXPRESSION:
            return "missing closing parenthesis";

        case EVAL_ERROR_UNKNOWN_OPERAND:
            return "unknown operand";

        case EVAL_ERROR_MISSING_OPERAND:
            return "expected operand";

        case EVAL_ERROR_MISSING_OPERATOR:
            return "expected operator";

        case EVAL_ERROR_REMAINING_GARBAGE:
            return "remaining characters after expression";

        case EVAL_ERROR_INVALID_VALUE:
            return "invalid value structure - internal error";

        case EVAL_ERROR_INVALID_NUMBER_OF_OPERANDS:
            return "wrong number of operands for operation - internal error";

        case EVAL_ERROR_VALUE_IS_NAN:
            return "value is unset";

        case EVAL_ERROR_VALUE_IS_INFINITE:
            return "computed value is infinite";

        case EVAL_ERROR_UNKNOWN_VARIABLE:
            return "undefined variable";

        case EVAL_ERROR_INVALID_OPERAND:
            return "invalid operand";

        case EVAL_ERROR_INVALID_OPERATOR:
            return "invalid operator";

        case EVAL_ERROR_IF_THEN_ELSE_MISSING_ELSE:
            return "missing second sub-expression of inline conditional";

        default:
            return "unknown error";
    }
}

const char *expression_source(EVAL_EXPRESSION *expression) {
    if(!expression)
        return string2str(NULL);

    return string2str(expression->source);
}

const char *expression_parsed_as(EVAL_EXPRESSION *expression) {
    if(!expression)
        return string2str(NULL);

    return string2str(expression->parsed_as);
}

const char *expression_error_msg(EVAL_EXPRESSION *expression) {
    if(!expression || !expression->error_msg)
        return "";

    return buffer_tostring(expression->error_msg);
}

NETDATA_DOUBLE expression_result(EVAL_EXPRESSION *expression) {
    if(!expression)
        return NAN;

    return expression->result;
}

void expression_set_variable_lookup_callback(EVAL_EXPRESSION *expression, eval_expression_variable_lookup_t cb, void *data) {
    if(!expression)
        return;

    expression->variable_lookup_cb = cb;
    expression->variable_lookup_cb_data = data;
}

static size_t expression_hardcode_node_variable(EVAL_NODE *node, STRING *variable, NETDATA_DOUBLE value) {
    size_t matches = 0;

    for(int i = 0; i < node->count; i++) {
        switch(node->ops[i].type) {
            case EVAL_VALUE_NUMBER:
            case EVAL_VALUE_INVALID:
                break;

            case EVAL_VALUE_VARIABLE:
                if(node->ops[i].variable->name == variable) {
                    string_freez(node->ops[i].variable->name);
                    freez(node->ops[i].variable);
                    node->ops[i].type = EVAL_VALUE_NUMBER;
                    node->ops[i].number = value;
                    matches++;
                }
                break;

            case EVAL_VALUE_EXPRESSION:
                matches += expression_hardcode_node_variable(node->ops[i].expression, variable, value);
                break;
        }
    }

    return matches;
}

static size_t str_replace_cpy(char *dst, size_t dst_size,
                     const char *src,
                     const char *variable, size_t variable_len,
                     const char *value, size_t value_len) {

    if (!dst || !src || !variable || !value || dst_size == 0 || variable_len == 0 || value_len == 0)
        return 0;

    const char *pos = strstr(src, variable);
    if (!pos)
        return 0;

    size_t src_idx = 0;
    size_t dst_idx = 0;
    size_t matches = 0;

    while (src[src_idx] != '\0') {
        if (pos && &src[src_idx] == pos) {
            if (dst_idx + value_len >= dst_size)
                return 0;

            matches++;
            memcpy(&dst[dst_idx], value, value_len);
            dst_idx += value_len;
            src_idx += variable_len;

            pos = strstr(&src[src_idx], variable);
        } else {
            if (dst_idx + 1 >= dst_size)
                return 0;
            dst[dst_idx++] = src[src_idx++];
        }
    }

    if (dst_idx >= dst_size)
        return 0;

    dst[dst_idx] = '\0';

    return matches;
}


void expression_hardcode_variable(EVAL_EXPRESSION *expression, STRING *variable, NETDATA_DOUBLE value) {
    if (!expression || !variable)
        return;

    size_t matches = expression_hardcode_node_variable(expression->nodes, variable, value);
    if (matches) {
        char replace[DOUBLE_MAX_LENGTH];
        if(isnan(value))
            strncpyz(replace, "nan", sizeof(replace));
        else if(isinf(value))
            strncpyz(replace, "inf", sizeof(replace));
        else
            print_netdata_double(replace, value);
        size_t replace_len = strlen(replace);

        size_t source_len = string_strlen(expression->source);

        char find1[string_strlen(variable) + 1 + 1];
        snprintfz(find1, sizeof(find1), "$%s", string2str(variable));
        size_t find1_len = strlen(find1);

        char find2[string_strlen(variable) + 1 + 3];
        snprintfz(find2, sizeof(find2), "${%s}", string2str(variable));
        size_t find2_len = strlen(find2);

        // Calculate the maximum possible buffer size needed
        // Source length + (max replacement length - min variable length) * matches + null terminator
        size_t min_var_len = MIN(find1_len, find2_len);
        size_t max_buf_size = source_len + 1 + (matches * (replace_len > min_var_len ? replace_len - min_var_len : 0));

        char buf1[max_buf_size];
        char buf2[max_buf_size];

        char *dst[2] = {buf1, buf2};

        const char *src = string2str(expression->source);
        size_t slot = 0;
        while(matches) {
            size_t matched = 0;

            matched = str_replace_cpy(dst[slot], max_buf_size, src, find1, find1_len, replace, replace_len);
            if(matched) {
                src = dst[slot];
                if(++slot > 1) slot = 0;
                matches -= MIN(matches, matched);
            }

            if(matches) {
                matched = str_replace_cpy(dst[slot], max_buf_size, src, find2, find2_len, replace, replace_len);
                if(matched) {
                    src = dst[slot];
                    if (++slot > 1) slot = 0;
                    matches -= MIN(matches, matched);
                }
            }
        }

        // Update the expression source with the new string.
        string_freez(expression->source);
        expression->source = string_strdupz(src);
    }
}
