// SPDX-License-Identifier: GPL-3.0-or-later

#include "windows-events-xml.h"

#include <string.h>
#include <stdio.h>

#define INDENT_STEP 2
#define A_LOT_OF_SPACES "                                                                                            "

// Helper: Add indentation
static inline void buffer_add_xml_indent(BUFFER *buffer, const int level) {
    size_t total_spaces = (size_t)level * INDENT_STEP;
    const size_t step = sizeof(A_LOT_OF_SPACES) - 1;
    while (total_spaces > 0) {
        const size_t spaces_to_add = (total_spaces > step) ? step : total_spaces;
        buffer_fast_strcat(buffer, A_LOT_OF_SPACES, spaces_to_add);
        total_spaces -= spaces_to_add;
    }
}

const char *append_the_rest(BUFFER *buffer, const char *xml, const char *end) {
    if(xml >= end) return end;
    buffer_fast_strcat(buffer, xml, end - xml);
    return end;
}

static const char *parse_node(BUFFER *buffer, const char *xml, const char *end, int level);

// Helper: Parse the value (between > and <) and return the next position to parse
const char *parse_value_and_closing_tag(BUFFER *buffer, const char *xml, const char *end, int level) {
    const char *start = xml;
    bool has_subnodes = false;

    while (xml < end) {
        if(*xml == '<') {
            if(xml + 1 < end && *(xml + 1) == '/') {
                // a closing tag
                xml += 2;

                while(xml < end && *xml != '>')
                    xml++;

                if(xml < end && *xml == '>')
                    xml++;

                if(has_subnodes) {
                    buffer_putc(buffer, '\n');
                    buffer_add_xml_indent(buffer, level);
                }

                buffer_fast_strcat(buffer, start, xml - start);
                return xml;
            }
            else {
                // an opening tag
                buffer_fast_strcat(buffer, start, xml - start);
                xml = start = parse_node(buffer, xml, end, level + 1);
                while(xml < end && isspace(*xml))
                    xml++;
                has_subnodes = true;
            }
        }
        else
            xml++;
    }

    return append_the_rest(buffer, start, end);
}

// Parse a field value and return the next position to parse
const char *parse_field_value(BUFFER *buffer, const char *xml, const char *end) {
    const char quote = *xml;

    if(quote != '"' && quote != '\'')
        return append_the_rest(buffer, xml, end);

    const char *start = xml++;

    while (xml < end && *xml != quote) {
        if (*xml == '\\') {
            xml++;  // Skip escape character

            if(xml < end)
                xml++;

            continue;
        }

        xml++;
    }

    if(xml < end && *xml == quote) {
        xml++;  // Move past the closing quote
        buffer_fast_strcat(buffer, start, xml - start);
        return xml;
    }

    return append_the_rest(buffer, start, end);
}

// Parse a field name and return the next position to parse
const char *parse_field(BUFFER *buffer, const char *xml, const char *end) {
    while(isspace(*xml) && xml < end) xml++;

    const char *start = xml;

    while (*xml != '=' && xml < end)
        xml++;

    // Append the field name
    buffer_fast_strcat(buffer, start, xml - start);

    if(xml < end && *xml == '=') {
        xml++;

        buffer_putc(buffer, '=');

        if(xml < end && (*xml == '"' || *xml == '\''))
            xml = parse_field_value(buffer, xml, end);

        return xml;  // Return the next character to parse
    }

    return append_the_rest(buffer, start, end);
}

// Parse a node (handles fields and subnodes) and return the next position to parse
static inline const char *parse_node(BUFFER *buffer, const char *xml, const char *end, int level) {
    if(*xml != '<')
        return append_the_rest(buffer, xml, end);

    const char *start = xml++; // skip the <

    buffer_putc(buffer, '\n');
    buffer_add_xml_indent(buffer, level);

    // skip spaces before the tag name
    while(xml < end && isspace(*xml)) xml++;

    // Parse the tag name
    while (xml < end && *xml != '>' && *xml != '/') {
        xml++;

        if(xml < end && isspace(*xml)) {
            xml++;

            while(xml < end && isspace(*xml))
                xml++;

            if(xml < end && *xml == '/') {
               // an opening tag that is self-closing
                xml++;
                if(xml < end && *xml == '>') {
                    xml++;
                    buffer_fast_strcat(buffer, start, xml - start);
                    return xml;
                }
                else
                    return append_the_rest(buffer, start, end);
            }
            else if(xml < end && *xml == '>') {
                // the end of an opening tag
                xml++;
                buffer_fast_strcat(buffer, start, xml - start);
                return parse_value_and_closing_tag(buffer, xml, end, level);
            }
            else {
                buffer_fast_strcat(buffer, start, xml - start);
                xml = start = parse_field(buffer, xml, end);
                while(xml < end && isspace(*xml))
                    xml++;
            }
        }
    }

    bool self_closing_tag = false;
    if(xml < end && *xml == '/') {
        self_closing_tag = true;
        xml++;
    }

    if(xml < end && *xml == '>') {
        xml++;
        buffer_fast_strcat(buffer, start, xml - start);

        if(self_closing_tag)
            return xml;

        return parse_value_and_closing_tag(buffer, xml, end, level);
    }

    return append_the_rest(buffer, start, end);
}

// Main pretty-print XML function
void buffer_pretty_print_xml(BUFFER *buffer, const char *xml, size_t xml_len) {
    const char *end = xml + xml_len;

    while(xml < end) {
        while(xml < end && isspace(*xml))
            xml++;

        if(xml < end && *xml == '<')
            xml = parse_node(buffer, xml, end, 1);
        else {
            append_the_rest(buffer, xml, end);
            return;
        }
    }
}
