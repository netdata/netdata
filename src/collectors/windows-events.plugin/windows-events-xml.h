// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef WINDOWS_EVENTS_XML_H
#define WINDOWS_EVENTS_XML_H

#include "libnetdata/libnetdata.h"

void buffer_pretty_print_xml(BUFFER *buffer, const char *xml, size_t xml_len);
bool buffer_extract_and_print_xml(BUFFER *buffer, const char *xml, size_t xml_len, const char *prefix, const char *keys[]);
bool buffer_xml_extract_and_print_value(BUFFER *buffer, const char *xml, size_t xml_len, const char *prefix, const char *keys[]);

#endif //WINDOWS_EVENTS_XML_H
