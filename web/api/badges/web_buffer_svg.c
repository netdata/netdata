// SPDX-License-Identifier: GPL-3.0-or-later

#include "web_buffer_svg.h"

#define BADGE_HORIZONTAL_PADDING 4
#define VERDANA_KERNING 0.2
#define VERDANA_PADDING 1.0

/*
 * verdana11_widths[] has been generated with this method:
 * https://github.com/badges/shields/blob/master/measure-text.js
*/

static double verdana11_widths[128] = {
    [0] = 0.0,
    [1] = 0.0,
    [2] = 0.0,
    [3] = 0.0,
    [4] = 0.0,
    [5] = 0.0,
    [6] = 0.0,
    [7] = 0.0,
    [8] = 0.0,
    [9] = 0.0,
    [10] = 0.0,
    [11] = 0.0,
    [12] = 0.0,
    [13] = 0.0,
    [14] = 0.0,
    [15] = 0.0,
    [16] = 0.0,
    [17] = 0.0,
    [18] = 0.0,
    [19] = 0.0,
    [20] = 0.0,
    [21] = 0.0,
    [22] = 0.0,
    [23] = 0.0,
    [24] = 0.0,
    [25] = 0.0,
    [26] = 0.0,
    [27] = 0.0,
    [28] = 0.0,
    [29] = 0.0,
    [30] = 0.0,
    [31] = 0.0,
    [32] = 3.8671874999999996, //
    [33] = 4.3291015625, // !
    [34] = 5.048828125, // "
    [35] = 9.001953125, // #
    [36] = 6.9931640625, // $
    [37] = 11.837890625, // %
    [38] = 7.992187499999999, // &
    [39] = 2.9541015625, // '
    [40] = 4.9951171875, // (
    [41] = 4.9951171875, // )
    [42] = 6.9931640625, // *
    [43] = 9.001953125, // +
    [44] = 4.00146484375, // ,
    [45] = 4.9951171875, // -
    [46] = 4.00146484375, // .
    [47] = 4.9951171875, // /
    [48] = 6.9931640625, // 0
    [49] = 6.9931640625, // 1
    [50] = 6.9931640625, // 2
    [51] = 6.9931640625, // 3
    [52] = 6.9931640625, // 4
    [53] = 6.9931640625, // 5
    [54] = 6.9931640625, // 6
    [55] = 6.9931640625, // 7
    [56] = 6.9931640625, // 8
    [57] = 6.9931640625, // 9
    [58] = 4.9951171875, // :
    [59] = 4.9951171875, // ;
    [60] = 9.001953125, // <
    [61] = 9.001953125, // =
    [62] = 9.001953125, // >
    [63] = 5.99951171875, // ?
    [64] = 11.0, // @
    [65] = 7.51953125, // A
    [66] = 7.541015625, // B
    [67] = 7.680664062499999, // C
    [68] = 8.4755859375, // D
    [69] = 6.95556640625, // E
    [70] = 6.32177734375, // F
    [71] = 8.529296875, // G
    [72] = 8.26611328125, // H
    [73] = 4.6298828125, // I
    [74] = 5.00048828125, // J
    [75] = 7.62158203125, // K
    [76] = 6.123046875, // L
    [77] = 9.2705078125, // M
    [78] = 8.228515625, // N
    [79] = 8.658203125, // O
    [80] = 6.63330078125, // P
    [81] = 8.658203125, // Q
    [82] = 7.6484375, // R
    [83] = 7.51953125, // S
    [84] = 6.7783203125, // T
    [85] = 8.05126953125, // U
    [86] = 7.51953125, // V
    [87] = 10.87646484375, // W
    [88] = 7.53564453125, // X
    [89] = 6.767578125, // Y
    [90] = 7.53564453125, // Z
    [91] = 4.9951171875, // [
    [92] = 4.9951171875, // backslash
    [93] = 4.9951171875, // ]
    [94] = 9.001953125, // ^
    [95] = 6.9931640625, // _
    [96] = 6.9931640625, // `
    [97] = 6.6064453125, // a
    [98] = 6.853515625, // b
    [99] = 5.73095703125, // c
    [100] = 6.853515625, // d
    [101] = 6.552734375, // e
    [102] = 3.8671874999999996, // f
    [103] = 6.853515625, // g
    [104] = 6.9609375, // h
    [105] = 3.0185546875, // i
    [106] = 3.78662109375, // j
    [107] = 6.509765625, // k
    [108] = 3.0185546875, // l
    [109] = 10.69921875, // m
    [110] = 6.9609375, // n
    [111] = 6.67626953125, // o
    [112] = 6.853515625, // p
    [113] = 6.853515625, // q
    [114] = 4.6943359375, // r
    [115] = 5.73095703125, // s
    [116] = 4.33447265625, // t
    [117] = 6.9609375, // u
    [118] = 6.509765625, // v
    [119] = 9.001953125, // w
    [120] = 6.509765625, // x
    [121] = 6.509765625, // y
    [122] = 5.779296875, // z
    [123] = 6.982421875, // {
    [124] = 4.9951171875, // |
    [125] = 6.982421875, // }
    [126] = 9.001953125, // ~
    [127] = 0.0
};

// find the width of the string using the verdana 11points font
static inline double verdana11_width(const char *s, float em_size) {
    double w = 0.0;

    while(*s) {
        // if UTF8 multibyte char found and guess it's width equal 1em
        // as label width will be updated with JavaScript this is not so important

        // TODO: maybe move UTF8 functions from url.c to separate util in libnetdata
        //       then use url_utf8_get_byte_length etc.
        if(IS_UTF8_STARTBYTE(*s)) {
            s++;
            while(IS_UTF8_BYTE(*s) && !IS_UTF8_STARTBYTE(*s)){
                s++;
            }
            w += em_size;
        }
        else {
            if(likely(!(*s & 0x80))){ // Byte 1XXX XXXX is not valid in UTF8
            double t = verdana11_widths[(unsigned char)*s];
                if(t != 0.0)
                w += t + VERDANA_KERNING;
            }
            s++;
        }
    }

    w -= VERDANA_KERNING;
    w += VERDANA_PADDING;
    return w;
}

static inline size_t escape_xmlz(char *dst, const char *src, size_t len) {
    size_t i = len;

    // required escapes from
    // https://github.com/badges/shields/blob/master/badge.js
    while(*src && i) {
        switch(*src) {
            case '\\':
                *dst++ = '/';
                src++;
                i--;
                break;

            case '&':
                if(i > 5) {
                    strcpy(dst, "&amp;");
                    i -= 5;
                    dst += 5;
                    src++;
                }
                else goto cleanup;
                break;

            case '<':
                if(i > 4) {
                    strcpy(dst, "&lt;");
                    i -= 4;
                    dst += 4;
                    src++;
                }
                else goto cleanup;
                break;

            case '>':
                if(i > 4) {
                    strcpy(dst, "&gt;");
                    i -= 4;
                    dst += 4;
                    src++;
                }
                else goto cleanup;
                break;

            case '"':
                if(i > 6) {
                    strcpy(dst, "&quot;");
                    i -= 6;
                    dst += 6;
                    src++;
                }
                else goto cleanup;
                break;

            case '\'':
                if(i > 6) {
                    strcpy(dst, "&apos;");
                    i -= 6;
                    dst += 6;
                    src++;
                }
                else goto cleanup;
                break;

            default:
                i--;
                *dst++ = *src++;
                break;
        }
    }

cleanup:
    *dst = '\0';
    return len - i;
}

static inline char *format_value_with_precision_and_unit(char *value_string, size_t value_string_len,
    NETDATA_DOUBLE value, const char *units, int precision) {
    if(unlikely(isnan(value) || isinf(value)))
        value = 0.0;

    char *separator = "";
    if(unlikely(isalnum(*units)))
        separator = " ";

    if(precision < 0) {
        int len, lstop = 0, trim_zeros = 1;

        NETDATA_DOUBLE abs = value;
        if(isless(value, 0)) {
            lstop = 1;
            abs = fabsndd(value);
        }

        if(isgreaterequal(abs, 1000)) {
            len = snprintfz(value_string, value_string_len, "%0.0" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
            trim_zeros = 0;
        }
        else if(isgreaterequal(abs, 10))     len = snprintfz(value_string, value_string_len, "%0.1" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else if(isgreaterequal(abs, 1))      len = snprintfz(value_string, value_string_len, "%0.2" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else if(isgreaterequal(abs, 0.1))    len = snprintfz(value_string, value_string_len, "%0.2" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else if(isgreaterequal(abs, 0.01))   len = snprintfz(value_string, value_string_len, "%0.4" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else if(isgreaterequal(abs, 0.001))  len = snprintfz(value_string, value_string_len, "%0.5" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else if(isgreaterequal(abs, 0.0001)) len = snprintfz(value_string, value_string_len, "%0.6" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);
        else                                 len = snprintfz(value_string, value_string_len, "%0.7" NETDATA_DOUBLE_MODIFIER, (NETDATA_DOUBLE) value);

        if(unlikely(trim_zeros)) {
            int l;
            // remove trailing zeros from the decimal part
            for(l = len - 1; l > lstop; l--) {
                if(likely(value_string[l] == '0')) {
                    value_string[l] = '\0';
                    len--;
                }

                else if(unlikely(value_string[l] == '.')) {
                    value_string[l] = '\0';
                    len--;
                    break;
                }

                else
                    break;
            }
        }

        if(unlikely(len <= 0)) len = 1;
        snprintfz(&value_string[len], value_string_len - len, "%s%s", separator, units);
    }
    else {
        if(precision > 50) precision = 50;
        snprintfz(value_string, value_string_len, "%0.*" NETDATA_DOUBLE_MODIFIER "%s%s", precision, (NETDATA_DOUBLE) value, separator, units);
    }

    return value_string;
}

typedef enum badge_units_format {
    UNITS_FORMAT_NONE,
    UNITS_FORMAT_SECONDS,
    UNITS_FORMAT_SECONDS_AGO,
    UNITS_FORMAT_MINUTES,
    UNITS_FORMAT_MINUTES_AGO,
    UNITS_FORMAT_HOURS,
    UNITS_FORMAT_HOURS_AGO,
    UNITS_FORMAT_ONOFF,
    UNITS_FORMAT_UPDOWN,
    UNITS_FORMAT_OKERROR,
    UNITS_FORMAT_OKFAILED,
    UNITS_FORMAT_EMPTY,
    UNITS_FORMAT_PERCENT
} UNITS_FORMAT;


static struct units_formatter {
    const char *units;
    uint32_t hash;
    UNITS_FORMAT format;
} badge_units_formatters[] = {
        { "seconds",     0, UNITS_FORMAT_SECONDS },
        { "seconds ago", 0, UNITS_FORMAT_SECONDS_AGO },
        { "minutes",     0, UNITS_FORMAT_MINUTES },
        { "minutes ago", 0, UNITS_FORMAT_MINUTES_AGO },
        { "hours",       0, UNITS_FORMAT_HOURS },
        { "hours ago",   0, UNITS_FORMAT_HOURS_AGO },
        { "on/off",      0, UNITS_FORMAT_ONOFF },
        { "on-off",      0, UNITS_FORMAT_ONOFF },
        { "onoff",       0, UNITS_FORMAT_ONOFF },
        { "up/down",     0, UNITS_FORMAT_UPDOWN },
        { "up-down",     0, UNITS_FORMAT_UPDOWN },
        { "updown",      0, UNITS_FORMAT_UPDOWN },
        { "ok/error",    0, UNITS_FORMAT_OKERROR },
        { "ok-error",    0, UNITS_FORMAT_OKERROR },
        { "okerror",     0, UNITS_FORMAT_OKERROR },
        { "ok/failed",   0, UNITS_FORMAT_OKFAILED },
        { "ok-failed",   0, UNITS_FORMAT_OKFAILED },
        { "okfailed",    0, UNITS_FORMAT_OKFAILED },
        { "empty",       0, UNITS_FORMAT_EMPTY },
        { "null",        0, UNITS_FORMAT_EMPTY },
        { "percentage",  0, UNITS_FORMAT_PERCENT },
        { "percent",     0, UNITS_FORMAT_PERCENT },
        { "pcent",       0, UNITS_FORMAT_PERCENT },

        // terminator
        { NULL,          0, UNITS_FORMAT_NONE }
};

inline char *format_value_and_unit(char *value_string, size_t value_string_len,
    NETDATA_DOUBLE value, const char *units, int precision) {
    static int max = -1;
    int i;

    if(unlikely(max == -1)) {
        for(i = 0; badge_units_formatters[i].units; i++)
            badge_units_formatters[i].hash = simple_hash(badge_units_formatters[i].units);

        max = i;
    }

    if(unlikely(!units)) units = "";
    uint32_t hash_units = simple_hash(units);

    UNITS_FORMAT format = UNITS_FORMAT_NONE;
    for(i = 0; i < max; i++) {
        struct units_formatter *ptr = &badge_units_formatters[i];

        if(hash_units == ptr->hash && !strcmp(units, ptr->units)) {
            format = ptr->format;
            break;
        }
    }

    if(unlikely(format == UNITS_FORMAT_SECONDS || format == UNITS_FORMAT_SECONDS_AGO)) {
        if(value == 0.0) {
            snprintfz(value_string, value_string_len, "%s", "now");
            return value_string;
        }
        else if(isnan(value) || isinf(value)) {
            snprintfz(value_string, value_string_len, "%s", "undefined");
            return value_string;
        }

        const char *suffix = (format == UNITS_FORMAT_SECONDS_AGO)?" ago":"";

        size_t s = (size_t)value;
        size_t d = s / 86400;
        s = s % 86400;

        size_t h = s / 3600;
        s = s % 3600;

        size_t m = s / 60;
        s = s % 60;

        if(d)
            snprintfz(value_string, value_string_len, "%zu %s %02zu:%02zu:%02zu%s", d, (d == 1)?"day":"days", h, m, s, suffix);
        else
            snprintfz(value_string, value_string_len, "%02zu:%02zu:%02zu%s", h, m, s, suffix);

        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_MINUTES || format == UNITS_FORMAT_MINUTES_AGO)) {
        if(value == 0.0) {
            snprintfz(value_string, value_string_len, "%s", "now");
            return value_string;
        }
        else if(isnan(value) || isinf(value)) {
            snprintfz(value_string, value_string_len, "%s", "undefined");
            return value_string;
        }

        const char *suffix = (format == UNITS_FORMAT_MINUTES_AGO)?" ago":"";

        size_t m = (size_t)value;
        size_t d = m / (60 * 24);
        m = m % (60 * 24);

        size_t h = m / 60;
        m = m % 60;

        if(d)
            snprintfz(value_string, value_string_len, "%zud %02zuh %02zum%s", d, h, m, suffix);
        else
            snprintfz(value_string, value_string_len, "%zuh %zum%s", h, m, suffix);

        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_HOURS || format == UNITS_FORMAT_HOURS_AGO)) {
        if(value == 0.0) {
            snprintfz(value_string, value_string_len, "%s", "now");
            return value_string;
        }
        else if(isnan(value) || isinf(value)) {
            snprintfz(value_string, value_string_len, "%s", "undefined");
            return value_string;
        }

        const char *suffix = (format == UNITS_FORMAT_HOURS_AGO)?" ago":"";

        size_t h = (size_t)value;
        size_t d = h / 24;
        h = h % 24;

        if(d)
            snprintfz(value_string, value_string_len, "%zud %zuh%s", d, h, suffix);
        else
            snprintfz(value_string, value_string_len, "%zuh%s", h, suffix);

        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_ONOFF)) {
        snprintfz(value_string, value_string_len, "%s", (value != 0.0)?"on":"off");
        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_UPDOWN)) {
        snprintfz(value_string, value_string_len, "%s", (value != 0.0)?"up":"down");
        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_OKERROR)) {
        snprintfz(value_string, value_string_len, "%s", (value != 0.0)?"ok":"error");
        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_OKFAILED)) {
        snprintfz(value_string, value_string_len, "%s", (value != 0.0)?"ok":"failed");
        return value_string;
    }

    else if(unlikely(format == UNITS_FORMAT_EMPTY))
        units = "";

    else if(unlikely(format == UNITS_FORMAT_PERCENT))
        units = "%";

    if(unlikely(isnan(value) || isinf(value))) {
        strcpy(value_string, "-");
        return value_string;
    }

    return format_value_with_precision_and_unit(value_string, value_string_len, value, units, precision);
}

static struct badge_color {
    const char *name;
    uint32_t hash;
    const char *color;
} badge_colors[] = {

        // colors from:
        // https://github.com/badges/shields/blob/master/colorscheme.json

        { "brightgreen", 0, "4c1"    },
        { "green",       0, "97CA00" },
        { "yellow",      0, "dfb317" },
        { "yellowgreen", 0, "a4a61d" },
        { "orange",      0, "fe7d37" },
        { "red",         0, "e05d44" },
        { "blue",        0, "007ec6" },
        { "grey",        0, "555"    },
        { "gray",        0, "555"    },
        { "lightgrey",   0, "9f9f9f" },
        { "lightgray",   0, "9f9f9f" },

        // terminator
        { NULL,          0, NULL      }
};

static inline const char *color_map(const char *color, const char *def) {
    static int max = -1;
    int i;

    if(unlikely(max == -1)) {
        for(i = 0; badge_colors[i].name ;i++)
            badge_colors[i].hash = simple_hash(badge_colors[i].name);

        max = i;
    }

    uint32_t hash = simple_hash(color);

    for(i = 0; i < max; i++) {
        struct badge_color *ptr = &badge_colors[i];

        if(hash == ptr->hash && !strcmp(color, ptr->name))
            return ptr->color;
    }

    return def;
}

typedef enum color_comparison {
    COLOR_COMPARE_EQUAL,
    COLOR_COMPARE_NOTEQUAL,
    COLOR_COMPARE_LESS,
    COLOR_COMPARE_LESSEQUAL,
    COLOR_COMPARE_GREATER,
    COLOR_COMPARE_GREATEREQUAL,
} BADGE_COLOR_COMPARISON;

static inline void calc_colorz(const char *color, char *final, size_t len, NETDATA_DOUBLE value) {
    if(isnan(value) || isinf(value))
        value = NAN;

    char color_buffer[256 + 1] = "";
    char value_buffer[256 + 1] = "";
    BADGE_COLOR_COMPARISON comparison = COLOR_COMPARE_GREATER;

    // example input:
    // color<max|color>min|color:null...

    const char *c = color;
    while(*c) {
        char *dc = color_buffer, *dv = NULL;
        size_t ci = 0, vi = 0;

        const char *t = c;

        while(*t && *t != '|') {
            switch(*t) {
                case '!':
                    if(t[1] == '=') t++;
                    comparison = COLOR_COMPARE_NOTEQUAL;
                    dv = value_buffer;
                    break;

                case '=':
                case ':':
                    comparison = COLOR_COMPARE_EQUAL;
                    dv = value_buffer;
                    break;

                case '}':
                case ')':
                case '>':
                    if(t[1] == '=') {
                        comparison = COLOR_COMPARE_GREATEREQUAL;
                        t++;
                    }
                    else
                        comparison = COLOR_COMPARE_GREATER;
                    dv = value_buffer;
                    break;

                case '{':
                case '(':
                case '<':
                    if(t[1] == '=') {
                        comparison = COLOR_COMPARE_LESSEQUAL;
                        t++;
                    }
                    else if(t[1] == '>' || t[1] == ')' || t[1] == '}') {
                        comparison = COLOR_COMPARE_NOTEQUAL;
                        t++;
                    }
                    else
                        comparison = COLOR_COMPARE_LESS;
                    dv = value_buffer;
                    break;

                default:
                    if(dv) {
                        if(vi < 256) {
                            vi++;
                            *dv++ = *t;
                        }
                    }
                    else {
                        if(ci < 256) {
                            ci++;
                            *dc++ = *t;
                        }
                    }
                    break;
            }

            t++;
        }

        // prepare for next iteration
        if(*t == '|') t++;
        c = t;

        // do the math
        *dc = '\0';
        if(dv) {
            *dv = '\0';
            NETDATA_DOUBLE v;

            if(!*value_buffer || !strcmp(value_buffer, "null")) {
                v = NAN;
            }
            else {
                v = str2l(value_buffer);
                if(isnan(v) || isinf(v))
                    v = NAN;
            }

            if(unlikely(isnan(value) || isnan(v))) {
                if(isnan(value) && isnan(v))
                    break;
            }
            else {
                     if (unlikely(comparison == COLOR_COMPARE_LESS && isless(value, v))) break;
                else if (unlikely(comparison == COLOR_COMPARE_LESSEQUAL && islessequal(value, v))) break;
                else if (unlikely(comparison == COLOR_COMPARE_GREATER && isgreater(value, v))) break;
                else if (unlikely(comparison == COLOR_COMPARE_GREATEREQUAL && isgreaterequal(value, v))) break;
                else if (unlikely(comparison == COLOR_COMPARE_EQUAL && !islessgreater(value, v))) break;
                else if (unlikely(comparison == COLOR_COMPARE_NOTEQUAL && islessgreater(value, v))) break;
            }
        }
        else
            break;
    }

    const char *b;
    if(color_buffer[0])
        b = color_buffer;
    else
        b = color;

    strncpyz(final, b, len);
}

// value + units
#define VALUE_STRING_SIZE 100

// label
#define LABEL_STRING_SIZE 200

// colors
#define COLOR_STRING_SIZE 100

static inline int allowed_hexa_char(char x) {
    return ( (x >= '0' && x <= '9') ||
             (x >= 'a' && x <= 'f') ||
             (x >= 'A' && x <= 'F')
           );
}

static int html_color_check(const char *str) {
    int i = 0;
    while(str[i]) {
        if(!allowed_hexa_char(str[i]))
            return 0;
        if(unlikely(i >= 6))
            return 0;
        i++;
    }
    // want to allow either RGB or RRGGBB
    return ( i == 6 || i == 3 );
}

// Will parse color arg as #RRGGBB or #RGB or one of the colors
// from color_map hash table
// if parsing fails (argument error) it will return default color
// given as default parameter (def)
// in any case it will return either color in "RRGGBB" or "RGB" format as string
// or whatever is given as def (without checking - caller responsible to give sensible
// safely escaped default) as default if it fails
// in any case this function must always return something we can put directly in XML
// so no escaping is necessary anymore (with exception of default where caller is responsible)
// to give sensible default
#define BADGE_SVG_COLOR_ARG_MAXLEN 20

static const char *parse_color_argument(const char *arg, const char *def)
{
    if( !arg )
        return def;
    size_t len = strnlen(arg, BADGE_SVG_COLOR_ARG_MAXLEN);
    if( len < 2 || len >= BADGE_SVG_COLOR_ARG_MAXLEN )
        return def;
    if( html_color_check(arg) )
        return arg;
    return color_map(arg, def);
}

void buffer_svg(BUFFER *wb, const char *label,
    NETDATA_DOUBLE value, const char *units, const char *label_color, const char *value_color, int precision, int scale, uint32_t options, int fixed_width_lbl, int fixed_width_val, const char* text_color_lbl, const char* text_color_val) {
    char    value_color_buffer[COLOR_STRING_SIZE + 1]
            , value_string[VALUE_STRING_SIZE + 1]
            , label_escaped[LABEL_STRING_SIZE + 1]
            , value_escaped[VALUE_STRING_SIZE + 1];

    const char *label_color_parsed;
    const char *value_color_parsed;

    double label_width = (double)fixed_width_lbl, value_width = (double)fixed_width_val, total_width;
    double height = 20.0, font_size = 11.0, text_offset = 5.8, round_corner = 3.0;

    if(scale < 100) scale = 100;

    if(unlikely(!value_color || !*value_color))
        value_color = (isnan(value) || isinf(value))?"999":"4c1";

    calc_colorz(value_color, value_color_buffer, COLOR_STRING_SIZE, value);
    format_value_and_unit(value_string, VALUE_STRING_SIZE, (options & RRDR_OPTION_DISPLAY_ABS)? fabsndd(value):value, units, precision);

    if(fixed_width_lbl <= 0 || fixed_width_val <= 0) {
        label_width = verdana11_width(label, font_size) + (BADGE_HORIZONTAL_PADDING * 2);
        value_width = verdana11_width(value_string, font_size) + (BADGE_HORIZONTAL_PADDING * 2);
    }
    total_width = label_width + value_width;

    escape_xmlz(label_escaped, label, LABEL_STRING_SIZE);
    escape_xmlz(value_escaped, value_string, VALUE_STRING_SIZE);

    label_color_parsed = parse_color_argument(label_color, "555");
    value_color_parsed = parse_color_argument(value_color_buffer, "555");

    wb->contenttype = CT_IMAGE_SVG_XML;

    total_width  = total_width * scale / 100.0;
    height       = height      * scale / 100.0;
    font_size    = font_size   * scale / 100.0;
    text_offset  = text_offset * scale / 100.0;
    label_width  = label_width * scale / 100.0;
    value_width  = value_width * scale / 100.0;
    round_corner = round_corner * scale / 100.0;

    // svg template from:
    // https://raw.githubusercontent.com/badges/shields/master/templates/flat-template.svg
    buffer_sprintf(wb,
        "<svg xmlns=\"http://www.w3.org/2000/svg\" xmlns:xlink=\"http://www.w3.org/1999/xlink\" width=\"%0.2f\" height=\"%0.2f\">"
            "<linearGradient id=\"smooth\" x2=\"0\" y2=\"100%%\">"
                "<stop offset=\"0\" stop-color=\"#bbb\" stop-opacity=\".1\"/>"
                "<stop offset=\"1\" stop-opacity=\".1\"/>"
            "</linearGradient>"
            "<mask id=\"round\">"
                "<rect class=\"bdge-ttl-width\" width=\"%0.2f\" height=\"%0.2f\" rx=\"%0.2f\" fill=\"#fff\"/>"
            "</mask>"
            "<g mask=\"url(#round)\">"
                "<rect class=\"bdge-rect-lbl\" width=\"%0.2f\" height=\"%0.2f\" fill=\"#%s\"/>",
        total_width, height,
        total_width, height, round_corner,
        label_width, height, label_color_parsed); //<rect class="bdge-rect-lbl"

    if(fixed_width_lbl > 0 && fixed_width_val > 0) {
        buffer_sprintf(wb,
                "<clipPath id=\"lbl-rect\">"
                    "<rect class=\"bdge-rect-lbl\" width=\"%0.2f\" height=\"%0.2f\"/>"
                "</clipPath>",
        label_width, height); //<clipPath id="lbl-rect"> <rect class="bdge-rect-lbl"
    }

    buffer_sprintf(wb,
                "<rect class=\"bdge-rect-val\" x=\"%0.2f\" width=\"%0.2f\" height=\"%0.2f\" fill=\"#%s\"/>",
        label_width, value_width, height, value_color_parsed);
    
    if(fixed_width_lbl > 0 && fixed_width_val > 0) {
        buffer_sprintf(wb,
                "<clipPath id=\"val-rect\">"
                    "<rect class=\"bdge-rect-val\" x=\"%0.2f\" width=\"%0.2f\" height=\"%0.2f\"/>"
                "</clipPath>",
        label_width, value_width, height);
    }

    buffer_sprintf(wb,
                "<rect class=\"bdge-ttl-width\" width=\"%0.2f\" height=\"%0.2f\" fill=\"url(#smooth)\"/>"
            "</g>"
            "<g text-anchor=\"middle\" font-family=\"DejaVu Sans,Verdana,Geneva,sans-serif\" font-size=\"%0.2f\">"
                "<text class=\"bdge-lbl-lbl\" x=\"%0.2f\" y=\"%0.0f\" fill=\"#010101\" fill-opacity=\".3\" clip-path=\"url(#lbl-rect)\">%s</text>"
                "<text class=\"bdge-lbl-lbl\" x=\"%0.2f\" y=\"%0.0f\" fill=\"#%s\" clip-path=\"url(#lbl-rect)\">%s</text>"
                "<text class=\"bdge-lbl-val\" x=\"%0.2f\" y=\"%0.0f\" fill=\"#010101\" fill-opacity=\".3\" clip-path=\"url(#val-rect)\">%s</text>"
                "<text class=\"bdge-lbl-val\" x=\"%0.2f\" y=\"%0.0f\" fill=\"#%s\" clip-path=\"url(#val-rect)\">%s</text>"
            "</g>",
        total_width, height,
        font_size,
        label_width / 2, ceil(height - text_offset), label_escaped,
        label_width / 2, ceil(height - text_offset - 1.0), parse_color_argument(text_color_lbl, "fff"), label_escaped,
        label_width + value_width / 2 -1, ceil(height - text_offset), value_escaped,
        label_width + value_width / 2 -1, ceil(height - text_offset - 1.0), parse_color_argument(text_color_val, "fff"), value_escaped);

    if(fixed_width_lbl <= 0 || fixed_width_val <= 0){
        buffer_sprintf(wb,
            "<script type=\"text/javascript\">"
                "var bdg_horiz_padding = %d;"
                "function netdata_bdge_each(list, attr, value){"
                    "Array.prototype.forEach.call(list, function(el){"
                        "el.setAttribute(attr, value);"
                    "});"
                "};"
                "var this_svg = document.currentScript.closest(\"svg\");"
                "var elem_lbl = this_svg.getElementsByClassName(\"bdge-lbl-lbl\");"
                "var elem_val = this_svg.getElementsByClassName(\"bdge-lbl-val\");"
                "var lbl_size = elem_lbl[0].getBBox();"
                "var val_size = elem_val[0].getBBox();"
                "var width_total = lbl_size.width + bdg_horiz_padding*2;"
                "this_svg.getElementsByClassName(\"bdge-rect-lbl\")[0].setAttribute(\"width\", width_total);"
                "netdata_bdge_each(elem_lbl, \"x\", (lbl_size.width / 2) + bdg_horiz_padding);"
                "netdata_bdge_each(elem_val, \"x\", width_total + (val_size.width / 2) + bdg_horiz_padding);"
                "var val_rect = this_svg.getElementsByClassName(\"bdge-rect-val\")[0];"
                "val_rect.setAttribute(\"width\", val_size.width + bdg_horiz_padding*2);"
                "val_rect.setAttribute(\"x\", width_total);"
                "width_total += val_size.width + bdg_horiz_padding*2;"
                "var width_update_elems = this_svg.getElementsByClassName(\"bdge-ttl-width\");"
                "netdata_bdge_each(width_update_elems, \"width\", width_total);"
                "this_svg.setAttribute(\"width\", width_total);"
                "</script>",
            BADGE_HORIZONTAL_PADDING);
    }
    buffer_sprintf(wb, "</svg>");
}

#define BADGE_URL_ARG_LBL_COLOR "text_color_lbl"
#define BADGE_URL_ARG_VAL_COLOR "text_color_val"

int web_client_api_request_v1_badge(RRDHOST *host, struct web_client *w, char *url) {
    int ret = HTTP_RESP_BAD_REQUEST;
    buffer_flush(w->response.data);

    BUFFER *dimensions = NULL;

    const char *chart = NULL
    , *before_str = NULL
    , *after_str = NULL
    , *points_str = NULL
    , *multiply_str = NULL
    , *divide_str = NULL
    , *label = NULL
    , *units = NULL
    , *label_color = NULL
    , *value_color = NULL
    , *refresh_str = NULL
    , *precision_str = NULL
    , *scale_str = NULL
    , *alarm = NULL
    , *fixed_width_lbl_str = NULL
    , *fixed_width_val_str = NULL
    , *text_color_lbl_str = NULL
    , *text_color_val_str = NULL
    , *group_options = NULL;

    int group = RRDR_GROUPING_AVERAGE;
    uint32_t options = 0x00000000;

    while(url) {
        char *value = mystrsep(&url, "&");
        if(!value || !*value) continue;

        char *name = mystrsep(&value, "=");
        if(!name || !*name) continue;
        if(!value || !*value) continue;

        debug(D_WEB_CLIENT, "%llu: API v1 badge.svg query param '%s' with value '%s'", w->id, name, value);

        // name and value are now the parameters
        // they are not null and not empty

        if(!strcmp(name, "chart")) chart = value;
        else if(!strcmp(name, "dimension") || !strcmp(name, "dim") || !strcmp(name, "dimensions") || !strcmp(name, "dims")) {
            if(!dimensions)
                dimensions = buffer_create(100);

            buffer_strcat(dimensions, "|");
            buffer_strcat(dimensions, value);
        }
        else if(!strcmp(name, "after")) after_str = value;
        else if(!strcmp(name, "before")) before_str = value;
        else if(!strcmp(name, "points")) points_str = value;
        else if(!strcmp(name, "group_options")) group_options = value;
        else if(!strcmp(name, "group")) {
            group = web_client_api_request_v1_data_group(value, RRDR_GROUPING_AVERAGE);
        }
        else if(!strcmp(name, "options")) {
            options |= web_client_api_request_v1_data_options(value);
        }
        else if(!strcmp(name, "label")) label = value;
        else if(!strcmp(name, "units")) units = value;
        else if(!strcmp(name, "label_color")) label_color = value;
        else if(!strcmp(name, "value_color")) value_color = value;
        else if(!strcmp(name, "multiply")) multiply_str = value;
        else if(!strcmp(name, "divide")) divide_str = value;
        else if(!strcmp(name, "refresh")) refresh_str = value;
        else if(!strcmp(name, "precision")) precision_str = value;
        else if(!strcmp(name, "scale")) scale_str = value;
        else if(!strcmp(name, "fixed_width_lbl")) fixed_width_lbl_str = value;
        else if(!strcmp(name, "fixed_width_val")) fixed_width_val_str = value;
        else if(!strcmp(name, "alarm")) alarm = value;
        else if(!strcmp(name, BADGE_URL_ARG_LBL_COLOR)) text_color_lbl_str = value;
        else if(!strcmp(name, BADGE_URL_ARG_VAL_COLOR)) text_color_val_str = value;
    }

    int fixed_width_lbl = -1;
    int fixed_width_val = -1;

    if(fixed_width_lbl_str && *fixed_width_lbl_str
        && fixed_width_val_str && *fixed_width_val_str) {
        fixed_width_lbl = str2i(fixed_width_lbl_str);
        fixed_width_val = str2i(fixed_width_val_str);
    }

    if(!chart || !*chart) {
        buffer_no_cacheable(w->response.data);
        buffer_sprintf(w->response.data, "No chart id is given at the request.");
        goto cleanup;
    }

    int scale = (scale_str && *scale_str)?str2i(scale_str):100;

    RRDSET *st = rrdset_find(host, chart);
    if(!st) st = rrdset_find_byname(host, chart);
    if(!st) {
        buffer_no_cacheable(w->response.data);
        buffer_svg(w->response.data, "chart not found", NAN, "", NULL, NULL, -1, scale, 0, -1, -1, NULL, NULL);
        ret = HTTP_RESP_OK;
        goto cleanup;
    }
    st->last_accessed_time = now_realtime_sec();

    RRDCALC *rc = NULL;
    if(alarm) {
        rc = rrdcalc_find(st, alarm);
        if (!rc) {
            buffer_no_cacheable(w->response.data);
            buffer_svg(w->response.data, "alarm not found", NAN, "", NULL, NULL, -1, scale, 0, -1, -1, NULL, NULL);
            ret = HTTP_RESP_OK;
            goto cleanup;
        }
    }

    long long multiply  = (multiply_str  && *multiply_str )?str2l(multiply_str):1;
    long long divide    = (divide_str    && *divide_str   )?str2l(divide_str):1;
    long long before    = (before_str    && *before_str   )?str2l(before_str):0;
    long long after     = (after_str     && *after_str    )?str2l(after_str):-st->update_every;
    int       points    = (points_str    && *points_str   )?str2i(points_str):1;
    int       precision = (precision_str && *precision_str)?str2i(precision_str):-1;

    if(!multiply) multiply = 1;
    if(!divide) divide = 1;

    int refresh = 0;
    if(refresh_str && *refresh_str) {
        if(!strcmp(refresh_str, "auto")) {
            if(rc) refresh = rc->update_every;
            else if(options & RRDR_OPTION_NOT_ALIGNED)
                refresh = st->update_every;
            else {
                refresh = (int)(before - after);
                if(refresh < 0) refresh = -refresh;
            }
        }
        else {
            refresh = str2i(refresh_str);
            if(refresh < 0) refresh = -refresh;
        }
    }

    if(!label) {
        if(alarm) {
            char *s = (char *)alarm;
            while(*s) {
                if(*s == '_') *s = ' ';
                s++;
            }
            label = alarm;
        }
        else if(dimensions) {
            const char *dim = buffer_tostring(dimensions);
            if(*dim == '|') dim++;
            label = dim;
        }
        else
            label = rrdset_name(st);
    }
    if(!units) {
        if(alarm) {
            if(rc->units)
                units = rc->units;
            else
                units = "";
        }
        else if(options & RRDR_OPTION_PERCENTAGE)
            units = "%";
        else
            units = rrdset_units(st);
    }

    debug(D_WEB_CLIENT, "%llu: API command 'badge.svg' for chart '%s', alarm '%s', dimensions '%s', after '%lld', before '%lld', points '%d', group '%d', options '0x%08x'"
          , w->id
          , chart
          , alarm?alarm:""
          , (dimensions)?buffer_tostring(dimensions):""
          , after
          , before
          , points
          , group
          , options
    );

    if(rc) {
        if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        if(!value_color) {
            switch(rc->status) {
                case RRDCALC_STATUS_CRITICAL:
                    value_color = "red";
                    break;

                case RRDCALC_STATUS_WARNING:
                    value_color = "orange";
                    break;

                case RRDCALC_STATUS_CLEAR:
                    value_color = "brightgreen";
                    break;

                case RRDCALC_STATUS_UNDEFINED:
                    value_color = "lightgrey";
                    break;

                case RRDCALC_STATUS_UNINITIALIZED:
                    value_color = "#000";
                    break;

                default:
                    value_color = "grey";
                    break;
            }
        }

        buffer_svg(w->response.data,
                label,
                (isnan(rc->value)||isinf(rc->value)) ? rc->value : rc->value * multiply / divide,
                units,
                label_color,
                value_color,
                precision,
                scale,
                options,
                fixed_width_lbl,
                fixed_width_val,
                text_color_lbl_str,
                text_color_val_str
        );
        ret = HTTP_RESP_OK;
    }
    else {
        time_t latest_timestamp = 0;
        int value_is_null = 1;
        NETDATA_DOUBLE n = NAN;
        ret = HTTP_RESP_INTERNAL_SERVER_ERROR;

        // if the collected value is too old, don't calculate its value
        if (rrdset_last_entry_t(st) >= (now_realtime_sec() - (st->update_every * st->gap_when_lost_iterations_above)))
            ret = rrdset2value_api_v1(st, w->response.data, &n,
                                      (dimensions) ? buffer_tostring(dimensions) : NULL,
                                      points, after, before, group, group_options, 0, options,
                                      NULL, &latest_timestamp,
                                      NULL, NULL, NULL,
                                      &value_is_null, NULL, 0, 0);

        // if the value cannot be calculated, show empty badge
        if (ret != HTTP_RESP_OK) {
            buffer_no_cacheable(w->response.data);
            value_is_null = 1;
            n = 0;
            ret = HTTP_RESP_OK;
        }
        else if (refresh > 0) {
            buffer_sprintf(w->response.header, "Refresh: %d\r\n", refresh);
            w->response.data->expires = now_realtime_sec() + refresh;
        }
        else buffer_no_cacheable(w->response.data);

        // render the badge
        buffer_svg(w->response.data,
                label,
                (value_is_null)?NAN:(n * multiply / divide),
                units,
                label_color,
                value_color,
                precision,
                scale,
                options,
                fixed_width_lbl,
                fixed_width_val,
                text_color_lbl_str,
                text_color_val_str
        );
    }

    cleanup:
    buffer_free(dimensions);
    return ret;
}
