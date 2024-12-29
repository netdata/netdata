// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

ENUM_STR_MAP_DEFINE(HTTP_REQUEST_MODE) =
{
        { .name = "OPTIONS", .id = HTTP_REQUEST_MODE_OPTIONS },
        { .name = "GET", .id = HTTP_REQUEST_MODE_GET },
        { .name = "POST", .id = HTTP_REQUEST_MODE_POST },
        { .name = "PUT", .id = HTTP_REQUEST_MODE_PUT },
        { .name = "DELETE", .id = HTTP_REQUEST_MODE_DELETE },
        { .name = "STREAM", .id = HTTP_REQUEST_MODE_STREAM },

        // terminator
        { .name = NULL, .id = 0 }
};

ENUM_STR_DEFINE_FUNCTIONS(HTTP_REQUEST_MODE, 0, "UNKNOWN");

const char *http_response_code2string(int code) {
    switch(code) {
        case 100:
            return "Continue";
        case 101:
            return "Switching Protocols";
        case 102:
            return "Processing";
        case 103:
            return "Early Hints";

        case 200:
            return "OK";
        case 201:
            return "Created";
        case 202:
            return "Accepted";
        case 203:
            return "Non-Authoritative Information";
        case 204:
            return "No Content";
        case 205:
            return "Reset Content";
        case 206:
            return "Partial Content";
        case 207:
            return "Multi-Status";
        case 208:
            return "Already Reported";
        case 226:
            return "IM Used";

        case 300:
            return "Multiple Choices";
        case 301:
            return "Moved Permanently";
        case 302:
            return "Found";
        case 303:
            return "See Other";
        case 304:
            return "Not Modified";
        case 305:
            return "Use Proxy";
        case 306:
            return "Switch Proxy";
        case 307:
            return "Temporary Redirect";
        case 308:
            return "Permanent Redirect";

        case 400:
            return "Bad Request";
        case 401:
            return "Unauthorized";
        case 402:
            return "Payment Required";
        case 403:
            return "Forbidden";
        case 404:
            return "Not Found";
        case 405:
            return "Method Not Allowed";
        case 406:
            return "Not Acceptable";
        case 407:
            return "Proxy Authentication Required";
        case 408:
            return "Request Timeout";
        case 409:
            return "Conflict";
        case 410:
            return "Gone";
        case 411:
            return "Length Required";
        case 412:
            return "Precondition Failed";
        case 413:
            return "Payload Too Large";
        case 414:
            return "URI Too Long";
        case 415:
            return "Unsupported Media Type";
        case 416:
            return "Range Not Satisfiable";
        case 417:
            return "Expectation Failed";
        case 418:
            return "I'm a teapot";
        case 421:
            return "Misdirected Request";
        case 422:
            return "Unprocessable Entity";
        case 423:
            return "Locked";
        case 424:
            return "Failed Dependency";
        case 425:
            return "Too Early";
        case 426:
            return "Upgrade Required";
        case 428:
            return "Precondition Required";
        case 429:
            return "Too Many Requests";
        case 431:
            return "Request Header Fields Too Large";
        case 451:
            return "Unavailable For Legal Reasons";
        case 499: // nginx's extension to the standard
            return "Client Closed Request";

        case 500:
            return "Internal Server Error";
        case 501:
            return "Not Implemented";
        case 502:
            return "Bad Gateway";
        case 503:
            return "Service Unavailable";
        case 504:
            return "Gateway Timeout";
        case 505:
            return "HTTP Version Not Supported";
        case 506:
            return "Variant Also Negotiates";
        case 507:
            return "Insufficient Storage";
        case 508:
            return "Loop Detected";
        case 510:
            return "Not Extended";
        case 511:
            return "Network Authentication Required";

        default:
            if(code >= 100 && code < 200)
                return "Informational";

            if(code >= 200 && code < 300)
                return "Successful";

            if(code >= 300 && code < 400)
                return "Redirection";

            if(code >= 400 && code < 500)
                return "Client Error";

            if(code >= 500 && code < 600)
                return "Server Error";

            return "Undefined Error";
    }
}


static struct {
    const char *extension;
    uint32_t hash;
    HTTP_CONTENT_TYPE contenttype;
} mime_types[] = {
          { "html" , 0    , CT_TEXT_HTML }
        , { "js"   , 0    , CT_APPLICATION_X_JAVASCRIPT }
        , { "css"  , 0    , CT_TEXT_CSS }
        , { "xml"  , 0    , CT_TEXT_XML }
        , { "xsl"  , 0    , CT_TEXT_XSL }
        , { "txt"  , 0    , CT_TEXT_PLAIN }
        , { "svg"  , 0    , CT_IMAGE_SVG_XML }
        , { "ttf"  , 0    , CT_APPLICATION_X_FONT_TRUETYPE }
        , { "otf"  , 0    , CT_APPLICATION_X_FONT_OPENTYPE }
        , { "woff2", 0    , CT_APPLICATION_FONT_WOFF2 }
        , { "woff" , 0    , CT_APPLICATION_FONT_WOFF }
        , { "eot"  , 0    , CT_APPLICATION_VND_MS_FONTOBJ }
        , { "png"  , 0    , CT_IMAGE_PNG }
        , { "jpg"  , 0    , CT_IMAGE_JPG }
        , { "jpeg" , 0    , CT_IMAGE_JPG }
        , { "gif"  , 0    , CT_IMAGE_GIF }
        , { "bmp"  , 0    , CT_IMAGE_BMP }
        , { "ico"  , 0    , CT_IMAGE_XICON }
        , { "icns" , 0    , CT_IMAGE_ICNS }

        // terminator
        , { NULL   , 0    , 0 }
};

HTTP_CONTENT_TYPE contenttype_for_filename(const char *filename) {
    // netdata_log_info("checking filename '%s'", filename);

    static int initialized = 0;
    int i;

    if(unlikely(!initialized)) {
        for (i = 0; mime_types[i].extension; i++)
            mime_types[i].hash = simple_hash(mime_types[i].extension);

        initialized = 1;
    }

    const char *s = filename, *last_dot = NULL;

    // find the last dot
    while(*s) {
        if(unlikely(*s == '.')) last_dot = s;
        s++;
    }

    if(unlikely(!last_dot || !*last_dot || !last_dot[1])) {
        // netdata_log_info("no extension for filename '%s'", filename);
        return CT_APPLICATION_OCTET_STREAM;
    }
    last_dot++;

    // netdata_log_info("extension for filename '%s' is '%s'", filename, last_dot);

    uint32_t hash = simple_hash(last_dot);
    for(i = 0; mime_types[i].extension ; i++) {
        if(unlikely(hash == mime_types[i].hash && !strcmp(last_dot, mime_types[i].extension))) {
            // netdata_log_info("matched extension for filename '%s': '%s'", filename, last_dot);
            return mime_types[i].contenttype;
        }
    }

    // netdata_log_info("not matched extension for filename '%s': '%s'", filename, last_dot);
    return CT_APPLICATION_OCTET_STREAM;
}
