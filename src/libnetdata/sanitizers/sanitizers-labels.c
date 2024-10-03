// SPDX-License-Identifier: GPL-3.0-or-later

#include "sanitizers-labels.h"

/*
 * All labels follow these rules:
 *
 * Character           Symbol               Values     Names
 * UTF-8 characters    UTF-8                yes        -> _
 * Lower case letter   [a-z]                yes        yes
 * Upper case letter   [A-Z]                yes        -> [a-z]
 * Digit               [0-9]                yes        yes
 * Underscore          _                    yes        yes
 * Minus               -                    yes        yes
 * Plus                +                    yes        -> _
 * Colon               :                    yes        -> _
 * Semicolon           ;                    -> :       -> _
 * Equal               =                    -> :       -> _
 * Period              .                    yes        yes
 * Comma               ,                    -> .       -> .
 * Slash               /                    yes        yes
 * Backslash           \                    -> /       -> /
 * At                  @                    yes        -> _
 * Space                                    yes        -> _
 * Opening parenthesis (                    yes        -> _
 * Closing parenthesis )                    yes        -> _
 * anything else                            -> _       -> _
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

static unsigned char label_names_char_map[256];
static unsigned char label_values_char_map[256] = {
        [0] = '\0', [1] = '_', [2] = '_', [3] = '_', [4] = '_', [5] = '_', [6] = '_', [7] = '_', [8] = '_',

        // control characters to be treated as spaces
        ['\t'] = ' ', ['\n'] = ' ', ['\v'] = ' ', ['\f'] = ' ', ['\r'] = ' ',

        [14] = '_', [15] = '_', [16] = '_', [17] = '_', [18] = '_', [19] = '_', [20] = '_', [21] = '_',
        [22] = '_', [23] = '_', [24] = '_', [25] = '_', [26] = '_', [27] = '_', [28] = '_', [29] = '_',
        [30] = '_', [31] = '_',

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
        [127] = '_', [128] = '_', [129] = '_', [130] = '_', [131] = '_', [132] = '_', [133] = '_', [134] = '_',
        [135] = '_', [136] = '_', [137] = '_', [138] = '_', [139] = '_', [140] = '_', [141] = '_', [142] = '_',
        [143] = '_', [144] = '_', [145] = '_', [146] = '_', [147] = '_', [148] = '_', [149] = '_', [150] = '_',
        [151] = '_', [152] = '_', [153] = '_', [154] = '_', [155] = '_', [156] = '_', [157] = '_', [158] = '_',
        [159] = '_', [160] = '_', [161] = '_', [162] = '_', [163] = '_', [164] = '_', [165] = '_', [166] = '_',
        [167] = '_', [168] = '_', [169] = '_', [170] = '_', [171] = '_', [172] = '_', [173] = '_', [174] = '_',
        [175] = '_', [176] = '_', [177] = '_', [178] = '_', [179] = '_', [180] = '_', [181] = '_', [182] = '_',
        [183] = '_', [184] = '_', [185] = '_', [186] = '_', [187] = '_', [188] = '_', [189] = '_', [190] = '_',
        [191] = '_', [192] = '_', [193] = '_', [194] = '_', [195] = '_', [196] = '_', [197] = '_', [198] = '_',
        [199] = '_', [200] = '_', [201] = '_', [202] = '_', [203] = '_', [204] = '_', [205] = '_', [206] = '_',
        [207] = '_', [208] = '_', [209] = '_', [210] = '_', [211] = '_', [212] = '_', [213] = '_', [214] = '_',
        [215] = '_', [216] = '_', [217] = '_', [218] = '_', [219] = '_', [220] = '_', [221] = '_', [222] = '_',
        [223] = '_', [224] = '_', [225] = '_', [226] = '_', [227] = '_', [228] = '_', [229] = '_', [230] = '_',
        [231] = '_', [232] = '_', [233] = '_', [234] = '_', [235] = '_', [236] = '_', [237] = '_', [238] = '_',
        [239] = '_', [240] = '_', [241] = '_', [242] = '_', [243] = '_', [244] = '_', [245] = '_', [246] = '_',
        [247] = '_', [248] = '_', [249] = '_', [250] = '_', [251] = '_', [252] = '_', [253] = '_', [254] = '_',
        [255] = '_'
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
    label_names_char_map[' '] = '_';
    label_names_char_map['\\'] = '/';
}

size_t rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_names_char_map, 0, "", NULL);
}

size_t rrdlabels_sanitize_value(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_values_char_map, 1, "[none]", NULL);
}
