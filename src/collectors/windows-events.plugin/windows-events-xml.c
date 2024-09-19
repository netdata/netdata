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

    // const char *tag_start = NULL, *tag_end = NULL;
    while (xml < end) {
        if(*xml == '<') {
            if(xml + 1 < end && *(xml + 1) == '/') {
                // a closing tag
                xml += 2;

//                tag_start = xml;

                while(xml < end && *xml != '>')
                    xml++;

//                tag_end = xml;

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
                while(xml < end && isspace((uint8_t)*xml))
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
    while(isspace((uint8_t)*xml) && xml < end) xml++;

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
    while(xml < end && isspace((uint8_t)*xml)) xml++;

    // Parse the tag name
//    const char *tag_start = xml, *tag_end = NULL;
    while (xml < end && *xml != '>' && *xml != '/') {
        xml++;

        if(xml < end && isspace((uint8_t)*xml)) {
            xml++;
//            tag_end = xml;

            while(xml < end && isspace((uint8_t)*xml))
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
                while(xml < end && isspace((uint8_t)*xml))
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

static inline void buffer_pretty_print_xml_object(BUFFER *buffer, const char *xml, const char *end) {
    while(xml < end) {
        while(xml < end && isspace((uint8_t)*xml))
            xml++;

        if(xml < end && *xml == '<')
            xml = parse_node(buffer, xml, end, 1);
        else {
            append_the_rest(buffer, xml, end);
            return;
        }
    }
}

void buffer_pretty_print_xml(BUFFER *buffer, const char *xml, size_t xml_len) {
    const char *end = xml + xml_len;
    buffer_pretty_print_xml_object(buffer, xml, end);
}

// --------------------------------------------------------------------------------------------------------------------

bool buffer_extract_and_print_xml_with_cb(BUFFER *buffer, const char *xml, size_t xml_len, const char *prefix, const char *keys[],
                                          void (*cb)(BUFFER *, const char *, const char *, const char *)) {
    if(!keys || !*keys[0]) {
        buffer_pretty_print_xml(buffer, xml, xml_len);
        return true;
    }

    const char *start = xml, *end = NULL;
    for(size_t k = 0; keys[k] ; k++) {
        if(!*keys[k]) continue;

        size_t klen = strlen(keys[k]);
        char tag_open[klen + 2];
        tag_open[0] = '<';
        strcpy(&tag_open[1], keys[k]);
        tag_open[klen + 1] = '\0';

        const char *new_start = strstr(start, tag_open);
        if(!new_start)
            return false;

        start = new_start + klen + 1;

        if(*start != '>' && !isspace((uint8_t)*start))
            return false;

        if(*start != '>') {
            start = strchr(start, '>');
            if(!start) return false;
        }
        start++; // skip the >

        char tag_close[klen + 4];
        tag_close[0] = '<';
        tag_close[1] = '/';
        strcpy(&tag_close[2], keys[k]);
        tag_close[klen + 2] = '>';
        tag_close[klen + 3] = '\0';

        const char *new_end = strstr(start, tag_close);
        if(!new_end || (end && new_end > end))
            return false;

        end = new_end;
    }

    if(!start || !end || start == end)
        return false;

    cb(buffer, prefix, start, end);
    return true;
}

static void print_xml_cb(BUFFER *buffer, const char *prefix, const char *start, const char *end) {
    if(prefix)
        buffer_strcat(buffer, prefix);

    buffer_pretty_print_xml_object(buffer, start, end);
}

bool buffer_extract_and_print_xml(BUFFER *buffer, const char *xml, size_t xml_len, const char *prefix, const char *keys[]) {
    return buffer_extract_and_print_xml_with_cb(
        buffer, xml, xml_len,
        prefix, keys,
        print_xml_cb);
}

static void print_value_cb(BUFFER *buffer, const char *prefix, const char *start, const char *end) {
    if(prefix)
        buffer_strcat(buffer, prefix);

    buffer_need_bytes(buffer, end - start + 1);

    char *started = &buffer->buffer[buffer->len];
    char *d = started;
    const char *s = start;

    while(s < end && s) {
        if(*s == '&' && s + 3 < end) {
            if(*(s + 1) == '#') {
                if(s + 4 < end && *(s + 2) == '1' && *(s + 4) == ';') {
                    if (*(s + 3) == '0') {
                        s += 5;
                        *d++ = '\n';
                        continue;
                    } else if (*(s + 3) == '3') {
                        s += 5;
                        // *d++ = '\r';
                        continue;
                    }
                } else if (*(s + 2) == '9' && *(s + 3) == ';') {
                    s += 4;
                    *d++ = '\t';
                    continue;
                }
            }
            else if(s + 3 < end && *(s + 2) == 't' && *(s + 3) == ';') {
                if(*(s + 1) == 'l') {
                    s += 4;
                    *d++ = '<';
                    continue;
                }
                else if(*(s + 1) == 'g') {
                    s += 4;
                    *d++ = '>';
                    continue;
                }
            }
        }
        *d++ = *s++;
    }
    *d = '\0';
    buffer->len += d - started;
}

bool buffer_xml_extract_and_print_value(BUFFER *buffer, const char *xml, size_t xml_len, const char *prefix, const char *keys[]) {
    return buffer_extract_and_print_xml_with_cb(
        buffer, xml, xml_len,
        prefix, keys,
        print_value_cb);
}
