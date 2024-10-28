// SPDX-License-Identifier: GPL-3.0-or-later

#include "sanitizers-labels.h"

/*
 * All labels follow these rules:
 *
 * Character           Symbol   Names     Values
 * UTF-8 characters    UTF-8    -> _      yes
 * Lower case letter   [a-z]    yes       yes
 * Upper case letter   [A-Z]    yes       yes
 * Digit               [0-9]    yes       yes
 * Underscore          _        yes       yes
 * Minus               -        yes       yes
 * Plus                +        -> _      yes
 * Colon               :        -> _      yes
 * Semicolon           ;        -> _      -> :
 * Equal               =        -> _      -> :
 * Period              .        yes       yes
 * Comma               ,        -> .      -> .
 * Slash               /        yes       yes
 * Backslash           \        -> /      -> /
 * At                  @        -> _      yes
 * Space                        -> _      yes
 * Opening parenthesis (        -> _      yes
 * Closing parenthesis )        -> _      yes
 * anything else                -> _      -> space
*
 * The above rules should allow users to set in tags (indicative):
 *
 * 1. hostnames and domain names as-is
 * 2. email addresses as-is
 * 3. floating point numbers, converted to always use a dot as the decimal point
 *
 * Leading and trailing spaces and control characters are removed from both label
 * names and values.
 *
 * Multiple spaces inside the label name or the value are removed (only 1 is retained).
 * In names spaces are also converted to underscores.
 *
 * Names that are only underscores are rejected (they do not enter the dictionary).
 *
 * The above rules do not require any conversion to be included in JSON strings.
 *
 * Label names and values are truncated to LABELS_MAX_LENGTH (200) characters.
 *
 * When parsing, label key and value are separated by the first colon (:) found.
 * So label:value1:value2 is parsed as key = "label", value = "value1:value2"
 *
 * This means a label key cannot contain a colon (:) - it is converted to
 * underscore if it does.
 *
 */

static unsigned char prometheus_label_names_char_map[256];
static unsigned char label_names_char_map[256];
static unsigned char label_values_char_map[256] = {
        [0] = '\0', [1] = ' ', [2] = ' ', [3] = ' ', [4] = ' ', [5] = ' ', [6] = ' ', [7] = ' ', [8] = ' ',

        // control characters to be treated as spaces
        ['\t'] = ' ', ['\n'] = ' ', ['\v'] = ' ', ['\f'] = ' ', ['\r'] = ' ',

        [14] = ' ', [15] = ' ', [16] = ' ', [17] = ' ', [18] = ' ', [19] = ' ', [20] = ' ', [21] = ' ',
        [22] = ' ', [23] = ' ', [24] = ' ', [25] = ' ', [26] = ' ', [27] = ' ', [28] = ' ', [29] = ' ',
        [30] = ' ', [31] = ' ',

        // symbols
        [' '] = ' ', ['!'] = '_', ['"'] = '_', ['#'] = '_', ['$'] = '_', ['%'] = '_', ['&'] = '_', ['\''] = '_',
        ['('] = '(', [')'] = ')', ['*'] = '_', ['+'] = '+', [','] = '.', ['-'] = '-', ['.'] = '.', ['/'] = '/',

        // numbers
        ['0'] = '0', ['1'] = '1', ['2'] = '2', ['3'] = '3', ['4'] = '4', ['5'] = '5', ['6'] = '6', ['7'] = '7',
        ['8'] = '8', ['9'] = '9',

        // symbols
        [':'] = ':', [';'] = ':', ['<'] = '_', ['='] = ':', ['>'] = '_', ['?'] = '_', ['@'] = '@',

        // capitals
        ['A'] = 'A', ['B'] = 'B', ['C'] = 'C', ['D'] = 'D', ['E'] = 'E', ['F'] = 'F', ['G'] = 'G', ['H'] = 'H',
        ['I'] = 'I', ['J'] = 'J', ['K'] = 'K', ['L'] = 'L', ['M'] = 'M', ['N'] = 'N', ['O'] = 'O', ['P'] = 'P',
        ['Q'] = 'Q', ['R'] = 'R', ['S'] = 'S', ['T'] = 'T', ['U'] = 'U', ['V'] = 'V', ['W'] = 'W', ['X'] = 'X',
        ['Y'] = 'Y', ['Z'] = 'Z',

        // symbols
        ['['] = '[', ['\\'] = '/', [']'] = ']', ['^'] = '_', ['_'] = '_', ['`'] = '_',

        // lower
        ['a'] = 'a', ['b'] = 'b', ['c'] = 'c', ['d'] = 'd', ['e'] = 'e', ['f'] = 'f', ['g'] = 'g', ['h'] = 'h',
        ['i'] = 'i', ['j'] = 'j', ['k'] = 'k', ['l'] = 'l', ['m'] = 'm', ['n'] = 'n', ['o'] = 'o', ['p'] = 'p',
        ['q'] = 'q', ['r'] = 'r', ['s'] = 's', ['t'] = 't', ['u'] = 'u', ['v'] = 'v', ['w'] = 'w', ['x'] = 'x',
        ['y'] = 'y', ['z'] = 'z',

        // symbols
        ['{'] = '_', ['|'] = '_', ['}'] = '_', ['~'] = '_',

        // rest
        [127] = ' ', [128] = ' ', [129] = ' ', [130] = ' ', [131] = ' ', [132] = ' ', [133] = ' ', [134] = ' ',
        [135] = ' ', [136] = ' ', [137] = ' ', [138] = ' ', [139] = ' ', [140] = ' ', [141] = ' ', [142] = ' ',
        [143] = ' ', [144] = ' ', [145] = ' ', [146] = ' ', [147] = ' ', [148] = ' ', [149] = ' ', [150] = ' ',
        [151] = ' ', [152] = ' ', [153] = ' ', [154] = ' ', [155] = ' ', [156] = ' ', [157] = ' ', [158] = ' ',
        [159] = ' ', [160] = ' ', [161] = ' ', [162] = ' ', [163] = ' ', [164] = ' ', [165] = ' ', [166] = ' ',
        [167] = ' ', [168] = ' ', [169] = ' ', [170] = ' ', [171] = ' ', [172] = ' ', [173] = ' ', [174] = ' ',
        [175] = ' ', [176] = ' ', [177] = ' ', [178] = ' ', [179] = ' ', [180] = ' ', [181] = ' ', [182] = ' ',
        [183] = ' ', [184] = ' ', [185] = ' ', [186] = ' ', [187] = ' ', [188] = ' ', [189] = ' ', [190] = ' ',
        [191] = ' ', [192] = ' ', [193] = ' ', [194] = ' ', [195] = ' ', [196] = ' ', [197] = ' ', [198] = ' ',
        [199] = ' ', [200] = ' ', [201] = ' ', [202] = ' ', [203] = ' ', [204] = ' ', [205] = ' ', [206] = ' ',
        [207] = ' ', [208] = ' ', [209] = ' ', [210] = ' ', [211] = ' ', [212] = ' ', [213] = ' ', [214] = ' ',
        [215] = ' ', [216] = ' ', [217] = ' ', [218] = ' ', [219] = ' ', [220] = ' ', [221] = ' ', [222] = ' ',
        [223] = ' ', [224] = ' ', [225] = ' ', [226] = ' ', [227] = ' ', [228] = ' ', [229] = ' ', [230] = ' ',
        [231] = ' ', [232] = ' ', [233] = ' ', [234] = ' ', [235] = ' ', [236] = ' ', [237] = ' ', [238] = ' ',
        [239] = ' ', [240] = ' ', [241] = ' ', [242] = ' ', [243] = ' ', [244] = ' ', [245] = ' ', [246] = ' ',
        [247] = ' ', [248] = ' ', [249] = ' ', [250] = ' ', [251] = ' ', [252] = ' ', [253] = ' ', [254] = ' ',
        [255] = ' '
};

__attribute__((constructor)) void initialize_labels_keys_char_map(void) {
    // copy the values char map to the names char map
    size_t i;
    for(i = 0; i < 256 ;i++)
        label_names_char_map[i] = label_values_char_map[i];

    // apply overrides to the label names map
    label_names_char_map['='] = '_';
    label_names_char_map[':'] = '_';
    label_names_char_map['+'] = '_';
    label_names_char_map[';'] = '_';
    label_names_char_map['@'] = '_';
    label_names_char_map['('] = '_';
    label_names_char_map[')'] = '_';
    label_names_char_map['\\'] = '/';

    // prometheus label names
    for(i = 0; i < 256 ;i++) prometheus_label_names_char_map[i] = '_';
    for(int s = 'A' ; s <= 'Z' ; s++) prometheus_label_names_char_map[s] = s;
    for(int s = 'a' ; s <= 'z' ; s++) prometheus_label_names_char_map[s] = s;
    for(int s = '0' ; s <= '9' ; s++) prometheus_label_names_char_map[s] = s;
    prometheus_label_names_char_map[0] = '\0';
    prometheus_label_names_char_map[':'] = ':';
    prometheus_label_names_char_map['_'] = '_';
}

size_t rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size) {
    size_t rc = text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_names_char_map, 0, "", NULL);

    for(size_t i = 0; i < rc ; i++)
        if(dst[i] == ' ') dst[i] = '_';

    return rc;
}

size_t rrdlabels_sanitize_value(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_values_char_map, 1, "[none]", NULL);
}

size_t prometheus_rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, prometheus_label_names_char_map, 0, "", NULL);
}
