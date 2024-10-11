// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

/*
 * control characters become space, which are deduplicated.
 *
 *  Character Name   Sym      To  Why
 *  ---------------- ---      --- -------------------------------------------------------------------------------------
 *  space            [ ]  ->  [_]
 *  exclamation mark [!]  ->  [_] (only when it is the first character) simple patterns negation
 *  double quotes    ["]  ->  [_] needs escaping when parsing
 *  dollar           [$]  ->  [_] health variables and security in alarm-notify.sh, cgroup-name.sh, etc.
 *  percent          [%]  ->  [_] http GET encoded characters
 *  ampersand        [&]  ->  [_] http GET fields separator
 *  single quote     [']  ->  [_] needs escaping when parsing
 *  asterisk         [*]  ->  [_] simple pattern wildcard
 *  plus             [+]  ->  [_] http GET space
 *  comma            [,]  ->  [.] list separator (probably not used today)
 *  equal            [=]  ->  [_] plugins.d protocol separator
 *  question mark    [?]  ->  [_] http GET query string separator
 *  at               [@]  ->  [_] hostname separator (on the UI)
 *  apostrophe       [`]  ->  [_] bash expansion (security in alarm-notify.sh and other shell scripts)
 *  pipe             [|]  ->  [_] list separator (simple patterns and http GET)
 *  backslash        [\]  ->  [/] to avoid interfering with escaping logic
 */

unsigned char chart_names_allowed_chars[256] = {
    [0] = '\0', [1] = ' ', [2] = ' ', [3] = ' ', [4] = ' ', [5] = ' ', [6] = ' ', [7] = ' ', [8] = ' ',

    // control characters to be treated as spaces
    ['\t'] = ' ', ['\n'] = ' ', ['\v'] = ' ', ['\f'] = ' ', ['\r'] = ' ',

    [14] = ' ', [15] = ' ', [16] = ' ', [17] = ' ', [18] = ' ', [19] = ' ', [20] = ' ', [21] = ' ',
    [22] = ' ', [23] = ' ', [24] = ' ', [25] = ' ', [26] = ' ', [27] = ' ', [28] = ' ', [29] = ' ',
    [30] = ' ', [31] = ' ',

    // symbols
    [' '] = ' ', ['!'] = '!', ['"'] = '_', ['#'] = '#', ['$'] = '_', ['%'] = '_', ['&'] = '_', ['\''] = '_',
    ['('] = '(', [')'] = ')', ['*'] = '_', ['+'] = '_', [','] = '.', ['-'] = '-', ['.'] = '.', ['/'] = '/',

    // numbers
    ['0'] = '0', ['1'] = '1', ['2'] = '2', ['3'] = '3', ['4'] = '4', ['5'] = '5', ['6'] = '6', ['7'] = '7',
    ['8'] = '8', ['9'] = '9',

    // symbols
    [':'] = ':', [';'] = ';', ['<'] = '<', ['='] = '_', ['>'] = '>', ['?'] = '_', ['@'] = '_',

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
    ['{'] = '{', ['|'] = '_', ['}'] = '}', ['~'] = '~',

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

static inline void sanitize_chart_name(char *dst, const char *src, size_t dst_size) {
    // text_sanitize deduplicates spaces
    text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size,
                  chart_names_allowed_chars, true, "", NULL);

    char *d = dst;

    // do not accept ! as the first character
    if(*d == '!') *d = '_';

    // convert remaining spaces to underscores
    while(*d) {
        if(*d == ' ') *d = '_';
        d++;
    }
}

// make sure the supplied string
// is good for a netdata chart/dimension ID/NAME
void netdata_fix_chart_name(char *s) {
    sanitize_chart_name(s, s, strlen(s) + 1);
}

void netdata_fix_chart_id(char *s) {
    sanitize_chart_name(s, s, strlen(s) + 1);
//    size_t len = strlen(s);
//    char buf[len + 1];
//
//    text_sanitize((unsigned char *)buf, (const unsigned char *)s, sizeof(buf),
//                  chart_names_allowed_chars, true, "", NULL);
//
//    if(memcmp(s, buf, sizeof(buf)) == 0)
//        // they are the same
//        return;
//
//    // they differ
//    XXH128_hash_t hash = XXH3_128bits(s, len);
//    ND_UUID *uuid = (ND_UUID *)&hash;
//    internal_fatal(sizeof(hash) != sizeof(ND_UUID), "XXH128 and ND_UUID do not have the same size");
//    buf[0] = 'x';
//    buf[1] = 'x';
//    buf[2] = 'h';
//    buf[3] = '_';
//    uuid_unparse_lower_compact(uuid->uuid, &buf[4]);
}

char *rrdset_strncpyz_name(char *dst, const char *src, size_t dst_size_minus_1) {
    // src starts with "type."
    sanitize_chart_name(dst, src, dst_size_minus_1 + 1);
    return dst;
}

bool rrdvar_fix_name(char *variable) {
    size_t len = strlen(variable);
    char buf[len + 1];
    memcpy(buf, variable, sizeof(buf));
    sanitize_chart_name(variable, variable, len + 1);
    return memcmp(buf, variable, sizeof(buf)) != 0;
}
