// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_CONTENT_TYPE_H
#define NETDATA_CONTENT_TYPE_H

typedef enum __attribute__ ((__packed__)) {
    CT_NONE = 0,
    CT_APPLICATION_JSON,
    CT_TEXT_PLAIN,
    CT_TEXT_HTML,
    CT_APPLICATION_X_JAVASCRIPT,
    CT_TEXT_CSS,
    CT_TEXT_XML,
    CT_APPLICATION_XML,
    CT_TEXT_XSL,
    CT_APPLICATION_OCTET_STREAM,
    CT_APPLICATION_X_FONT_TRUETYPE,
    CT_APPLICATION_X_FONT_OPENTYPE,
    CT_APPLICATION_FONT_WOFF,
    CT_APPLICATION_FONT_WOFF2,
    CT_APPLICATION_VND_MS_FONTOBJ,
    CT_IMAGE_SVG_XML,
    CT_IMAGE_PNG,
    CT_IMAGE_JPG,
    CT_IMAGE_GIF,
    CT_IMAGE_XICON,
    CT_IMAGE_ICNS,
    CT_IMAGE_BMP,
    CT_PROMETHEUS,
    CT_AUDIO_MPEG,
    CT_AUDIO_OGG,
    CT_VIDEO_MP4,
    CT_APPLICATION_PDF,
    CT_APPLICATION_ZIP,
    CT_TEXT_YAML,
} HTTP_CONTENT_TYPE;

HTTP_CONTENT_TYPE content_type_string2id(const char *format);
const char *content_type_id2string(HTTP_CONTENT_TYPE content_type);

#include "../libnetdata.h"

void http_header_content_type(struct web_buffer *wb, HTTP_CONTENT_TYPE type);

#endif //NETDATA_CONTENT_TYPE_H
