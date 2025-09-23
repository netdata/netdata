// SPDX-License-Identifier: GPL-3.0-or-later

#include "content_type.h"


static struct {
    const char *format;
    HTTP_CONTENT_TYPE content_type;
    bool needs_charset;
    const char *options;
} content_types[] = {
    // primary - preferred during id-to-string conversions
    { .format = "application/json",             CT_APPLICATION_JSON, true },
    { .format = "text/plain",                   CT_TEXT_PLAIN, true },
    { .format = "text/event-stream",            CT_TEXT_EVENT_STREAM, true },
    { .format = "text/html",                    CT_TEXT_HTML, true },
    { .format = "text/css",                     CT_TEXT_CSS, true },
    { .format = "text/yaml",                    CT_TEXT_YAML, true },
    { .format = "application/yaml",             CT_APPLICATION_YAML, true },
    { .format = "text/xml",                     CT_TEXT_XML, true },
    { .format = "text/xsl",                     CT_TEXT_XSL, true },
    { .format = "application/xml",              CT_APPLICATION_XML, true },
    { .format = "application/javascript",       CT_APPLICATION_X_JAVASCRIPT, true },
    { .format = "application/octet-stream",     CT_APPLICATION_OCTET_STREAM, false },
    { .format = "image/svg+xml",                CT_IMAGE_SVG_XML, false },
    { .format = "application/x-font-truetype",  CT_APPLICATION_X_FONT_TRUETYPE, false },
    { .format = "application/x-font-opentype",  CT_APPLICATION_X_FONT_OPENTYPE, false },
    { .format = "application/font-woff",        CT_APPLICATION_FONT_WOFF, false },
    { .format = "application/font-woff2",       CT_APPLICATION_FONT_WOFF2, false },
    { .format = "application/vnd.ms-fontobject",CT_APPLICATION_VND_MS_FONTOBJ, false },
    { .format = "image/png",                    CT_IMAGE_PNG, false },
    { .format = "image/jpeg",                   CT_IMAGE_JPG, false },
    { .format = "image/gif",                    CT_IMAGE_GIF, false },
    { .format = "image/x-icon",                 CT_IMAGE_XICON, false },
    { .format = "image/bmp",                    CT_IMAGE_BMP, false },
    { .format = "image/icns",                   CT_IMAGE_ICNS, false },
    { .format = "audio/mpeg",                   CT_AUDIO_MPEG, false },
    { .format = "audio/ogg",                    CT_AUDIO_OGG, false },
    { .format = "video/mp4",                    CT_VIDEO_MP4, false },
    { .format = "application/pdf",              CT_APPLICATION_PDF, false },
    { .format = "application/zip",              CT_APPLICATION_ZIP, false },
    { .format = "image/png",                    CT_IMAGE_PNG, false },

    // secondary - overlapping with primary

    { .format = "text/plain",                   CT_PROMETHEUS, true, "version=0.0.4" },
    { .format = "prometheus",                   CT_PROMETHEUS, true },
    { .format = "text",                         CT_TEXT_PLAIN, true },
    { .format = "txt",                          CT_TEXT_PLAIN, true },
    { .format = "json",                         CT_APPLICATION_JSON, true },
    { .format = "html",                         CT_TEXT_HTML, true },
    { .format = "xml",                          CT_APPLICATION_XML, true },

    // terminator
    { .format = NULL,                           CT_TEXT_PLAIN, true },
};

HTTP_CONTENT_TYPE content_type_string2id(const char *format) {
    if(format && *format) {
        for (int i = 0; content_types[i].format; i++)
            if (strcmp(content_types[i].format, format) == 0)
                return content_types[i].content_type;
    }

    return CT_TEXT_PLAIN;
}

const char *content_type_id2string(HTTP_CONTENT_TYPE content_type) {
    for (int i = 0; content_types[i].format; i++)
        if (content_types[i].content_type == content_type)
            return content_types[i].format;

    return "text/plain";
}

void http_header_content_type(BUFFER *wb, HTTP_CONTENT_TYPE content_type) {
    buffer_strcat(wb, "Content-Type: ");

    for (int i = 0; content_types[i].format; i++) {
        if (content_types[i].content_type == content_type) {
            buffer_strcat(wb, content_types[i].format);

            if(content_types[i].needs_charset) {
                buffer_strcat(wb, "; charset=utf-8");
            }
            if(content_types[i].options) {
                buffer_strcat(wb, "; ");
                buffer_strcat(wb, content_types[i].options);
            }

            buffer_strcat(wb, "\r\n");

            return;
        }
    }

    buffer_strcat(wb, "text/plain; charset=utf-8\r\n");
}
