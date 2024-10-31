// SPDX-License-Identifier: GPL-3.0-or-later

#include "log2journal.h"

void replace_node_free(REPLACE_NODE *rpn) {
    hashed_key_cleanup(&rpn->name);
    rpn->next = NULL;
    freez(rpn);
}

void replace_pattern_cleanup(REPLACE_PATTERN *rp) {
    if(rp->pattern) {
        freez((void *)rp->pattern);
        rp->pattern = NULL;
    }

    while(rp->nodes) {
        REPLACE_NODE *rpn = rp->nodes;
        rp->nodes = rpn->next;
        replace_node_free(rpn);
    }
}

static REPLACE_NODE *replace_pattern_add_node(REPLACE_NODE **head, bool is_variable, const char *text) {
    REPLACE_NODE *new_node = callocz(1, sizeof(REPLACE_NODE));
    if (!new_node)
        return NULL;

    hashed_key_set(&new_node->name, text, -1);
    new_node->is_variable = is_variable;
    new_node->next = NULL;

    if (*head == NULL)
        *head = new_node;

    else {
        REPLACE_NODE *current = *head;

        // append it
        while (current->next != NULL)
            current = current->next;

        current->next = new_node;
    }

    return new_node;
}

bool replace_pattern_set(REPLACE_PATTERN *rp, const char *pattern) {
    replace_pattern_cleanup(rp);

    rp->pattern = strdupz(pattern);
    const char *current = rp->pattern;

    while (*current != '\0') {
        if (*current == '$' && *(current + 1) == '{') {
            // Start of a variable
            const char *end = strchr(current, '}');
            if (!end) {
                l2j_log("Error: Missing closing brace in replacement pattern: %s", rp->pattern);
                return false;
            }

            size_t name_length = end - current - 2; // Length of the variable name
            char *variable_name = strndupz(current + 2, name_length);
            if (!variable_name) {
                l2j_log("Error: Memory allocation failed for variable name.");
                return false;
            }

            REPLACE_NODE *node = replace_pattern_add_node(&(rp->nodes), true, variable_name);
            if (!node) {
                freez(variable_name);
                l2j_log("Error: Failed to add replacement node for variable.");
                return false;
            }
            freez(variable_name);

            current = end + 1; // Move past the variable
        }
        else {
            // Start of literal text
            const char *start = current;
            while (*current != '\0' && !(*current == '$' && *(current + 1) == '{')) {
                current++;
            }

            size_t text_length = current - start;
            char *text = strndupz(start, text_length);
            if (!text) {
                l2j_log("Error: Memory allocation failed for literal text.");
                return false;
            }

            REPLACE_NODE *node = replace_pattern_add_node(&(rp->nodes), false, text);
            if (!node) {
                freez(text);
                l2j_log("Error: Failed to add replacement node for text.");
                return false;
            }
            freez(text);
        }
    }

    for(REPLACE_NODE *node = rp->nodes; node; node = node->next) {
        if(node->is_variable) {
            rp->has_variables = true;
            break;
        }
    }

    return true;
}
