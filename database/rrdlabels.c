// SPDX-License-Identifier: GPL-3.0-or-later

#define NETDATA_RRD_INTERNALS
#include "rrd.h"

// ----------------------------------------------------------------------------
// labels sanitization

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

#define RRDLABELS_MAX_NAME_LENGTH 200
#define RRDLABELS_MAX_VALUE_LENGTH 800 // 800 in bytes, up to 200 UTF-8 characters

static unsigned char label_spaces_char_map[256];
static unsigned char label_names_char_map[256];
static unsigned char label_values_char_map[256] = {
    [0] = '\0', //
    [1] = '_', //
    [2] = '_', //
    [3] = '_', //
    [4] = '_', //
    [5] = '_', //
    [6] = '_', //
    [7] = '_', //
    [8] = '_', //
    [9] = '_', //
    [10] = '_', //
    [11] = '_', //
    [12] = '_', //
    [13] = '_', //
    [14] = '_', //
    [15] = '_', //
    [16] = '_', //
    [17] = '_', //
    [18] = '_', //
    [19] = '_', //
    [20] = '_', //
    [21] = '_', //
    [22] = '_', //
    [23] = '_', //
    [24] = '_', //
    [25] = '_', //
    [26] = '_', //
    [27] = '_', //
    [28] = '_', //
    [29] = '_', //
    [30] = '_', //
    [31] = '_', //
    [32] = ' ', // SPACE keep
    [33] = '_', // !
    [34] = '_', // "
    [35] = '_', // #
    [36] = '_', // $
    [37] = '_', // %
    [38] = '_', // &
    [39] = '_', // '
    [40] = '(', // ( keep
    [41] = ')', // ) keep
    [42] = '_', // *
    [43] = '+', // + keep
    [44] = '.', // , convert , to .
    [45] = '-', // - keep
    [46] = '.', // . keep
    [47] = '/', // / keep
    [48] = '0', // 0 keep
    [49] = '1', // 1 keep
    [50] = '2', // 2 keep
    [51] = '3', // 3 keep
    [52] = '4', // 4 keep
    [53] = '5', // 5 keep
    [54] = '6', // 6 keep
    [55] = '7', // 7 keep
    [56] = '8', // 8 keep
    [57] = '9', // 9 keep
    [58] = ':', // : keep
    [59] = ':', // ; convert ; to :
    [60] = '_', // <
    [61] = ':', // = convert = to :
    [62] = '_', // >
    [63] = '_', // ?
    [64] = '@', // @
    [65] = 'A', // A keep
    [66] = 'B', // B keep
    [67] = 'C', // C keep
    [68] = 'D', // D keep
    [69] = 'E', // E keep
    [70] = 'F', // F keep
    [71] = 'G', // G keep
    [72] = 'H', // H keep
    [73] = 'I', // I keep
    [74] = 'J', // J keep
    [75] = 'K', // K keep
    [76] = 'L', // L keep
    [77] = 'M', // M keep
    [78] = 'N', // N keep
    [79] = 'O', // O keep
    [80] = 'P', // P keep
    [81] = 'Q', // Q keep
    [82] = 'R', // R keep
    [83] = 'S', // S keep
    [84] = 'T', // T keep
    [85] = 'U', // U keep
    [86] = 'V', // V keep
    [87] = 'W', // W keep
    [88] = 'X', // X keep
    [89] = 'Y', // Y keep
    [90] = 'Z', // Z keep
    [91] = '[', // [ keep
    [92] = '/', // backslash convert \ to /
    [93] = ']', // ] keep
    [94] = '_', // ^
    [95] = '_', // _ keep
    [96] = '_', // `
    [97] = 'a', // a keep
    [98] = 'b', // b keep
    [99] = 'c', // c keep
    [100] = 'd', // d keep
    [101] = 'e', // e keep
    [102] = 'f', // f keep
    [103] = 'g', // g keep
    [104] = 'h', // h keep
    [105] = 'i', // i keep
    [106] = 'j', // j keep
    [107] = 'k', // k keep
    [108] = 'l', // l keep
    [109] = 'm', // m keep
    [110] = 'n', // n keep
    [111] = 'o', // o keep
    [112] = 'p', // p keep
    [113] = 'q', // q keep
    [114] = 'r', // r keep
    [115] = 's', // s keep
    [116] = 't', // t keep
    [117] = 'u', // u keep
    [118] = 'v', // v keep
    [119] = 'w', // w keep
    [120] = 'x', // x keep
    [121] = 'y', // y keep
    [122] = 'z', // z keep
    [123] = '_', // {
    [124] = '_', // |
    [125] = '_', // }
    [126] = '_', // ~
    [127] = '_', //
    [128] = '_', //
    [129] = '_', //
    [130] = '_', //
    [131] = '_', //
    [132] = '_', //
    [133] = '_', //
    [134] = '_', //
    [135] = '_', //
    [136] = '_', //
    [137] = '_', //
    [138] = '_', //
    [139] = '_', //
    [140] = '_', //
    [141] = '_', //
    [142] = '_', //
    [143] = '_', //
    [144] = '_', //
    [145] = '_', //
    [146] = '_', //
    [147] = '_', //
    [148] = '_', //
    [149] = '_', //
    [150] = '_', //
    [151] = '_', //
    [152] = '_', //
    [153] = '_', //
    [154] = '_', //
    [155] = '_', //
    [156] = '_', //
    [157] = '_', //
    [158] = '_', //
    [159] = '_', //
    [160] = '_', //
    [161] = '_', //
    [162] = '_', //
    [163] = '_', //
    [164] = '_', //
    [165] = '_', //
    [166] = '_', //
    [167] = '_', //
    [168] = '_', //
    [169] = '_', //
    [170] = '_', //
    [171] = '_', //
    [172] = '_', //
    [173] = '_', //
    [174] = '_', //
    [175] = '_', //
    [176] = '_', //
    [177] = '_', //
    [178] = '_', //
    [179] = '_', //
    [180] = '_', //
    [181] = '_', //
    [182] = '_', //
    [183] = '_', //
    [184] = '_', //
    [185] = '_', //
    [186] = '_', //
    [187] = '_', //
    [188] = '_', //
    [189] = '_', //
    [190] = '_', //
    [191] = '_', //
    [192] = '_', //
    [193] = '_', //
    [194] = '_', //
    [195] = '_', //
    [196] = '_', //
    [197] = '_', //
    [198] = '_', //
    [199] = '_', //
    [200] = '_', //
    [201] = '_', //
    [202] = '_', //
    [203] = '_', //
    [204] = '_', //
    [205] = '_', //
    [206] = '_', //
    [207] = '_', //
    [208] = '_', //
    [209] = '_', //
    [210] = '_', //
    [211] = '_', //
    [212] = '_', //
    [213] = '_', //
    [214] = '_', //
    [215] = '_', //
    [216] = '_', //
    [217] = '_', //
    [218] = '_', //
    [219] = '_', //
    [220] = '_', //
    [221] = '_', //
    [222] = '_', //
    [223] = '_', //
    [224] = '_', //
    [225] = '_', //
    [226] = '_', //
    [227] = '_', //
    [228] = '_', //
    [229] = '_', //
    [230] = '_', //
    [231] = '_', //
    [232] = '_', //
    [233] = '_', //
    [234] = '_', //
    [235] = '_', //
    [236] = '_', //
    [237] = '_', //
    [238] = '_', //
    [239] = '_', //
    [240] = '_', //
    [241] = '_', //
    [242] = '_', //
    [243] = '_', //
    [244] = '_', //
    [245] = '_', //
    [246] = '_', //
    [247] = '_', //
    [248] = '_', //
    [249] = '_', //
    [250] = '_', //
    [251] = '_', //
    [252] = '_', //
    [253] = '_', //
    [254] = '_', //
    [255] = '_'  //
};

__attribute__((constructor)) void initialize_labels_keys_char_map(void) {
    // copy the values char map to the names char map
    size_t i;
    for(i = 0; i < 256 ;i++)
        label_names_char_map[i] = label_values_char_map[i];

    // apply overrides to the label names map
    label_names_char_map['A'] = 'a';
    label_names_char_map['B'] = 'b';
    label_names_char_map['C'] = 'c';
    label_names_char_map['D'] = 'd';
    label_names_char_map['E'] = 'e';
    label_names_char_map['F'] = 'f';
    label_names_char_map['G'] = 'g';
    label_names_char_map['H'] = 'h';
    label_names_char_map['I'] = 'i';
    label_names_char_map['J'] = 'j';
    label_names_char_map['K'] = 'k';
    label_names_char_map['L'] = 'l';
    label_names_char_map['M'] = 'm';
    label_names_char_map['N'] = 'n';
    label_names_char_map['O'] = 'o';
    label_names_char_map['P'] = 'p';
    label_names_char_map['Q'] = 'q';
    label_names_char_map['R'] = 'r';
    label_names_char_map['S'] = 's';
    label_names_char_map['T'] = 't';
    label_names_char_map['U'] = 'u';
    label_names_char_map['V'] = 'v';
    label_names_char_map['W'] = 'w';
    label_names_char_map['X'] = 'x';
    label_names_char_map['Y'] = 'y';
    label_names_char_map['Z'] = 'z';
    label_names_char_map['='] = '_';
    label_names_char_map[':'] = '_';
    label_names_char_map['+'] = '_';
    label_names_char_map[';'] = '_';
    label_names_char_map['@'] = '_';
    label_names_char_map['('] = '_';
    label_names_char_map[')'] = '_';
    label_names_char_map[' '] = '_';
    label_names_char_map['\\'] = '/';

    // create the spaces map
    for(i = 0; i < 256 ;i++)
        label_spaces_char_map[i] = (isspace(i) || iscntrl(i) || !isprint(i))?1:0;

}

size_t text_sanitize(unsigned char *dst, const unsigned char *src, size_t dst_size, unsigned char *char_map, bool utf, const char *empty, size_t *multibyte_length) {
    if(unlikely(!dst_size)) return 0;

    if(unlikely(!src || !*src)) {
        strncpyz((char *)dst, empty, dst_size);
        dst[dst_size - 1] = '\0';
        size_t len = strlen((char *)dst);
        if(multibyte_length) *multibyte_length = len;
        return len;
    }

    unsigned char *d = dst;

    // make room for the final string termination
    unsigned char *end = &d[dst_size - 1];

    // copy while converting, but keep only one white space
    // we start wil last_is_space = 1 to skip leading spaces
    int last_is_space = 1;

    size_t mblen = 0;

    while(*src && d < end) {
        unsigned char c = *src;

        if(IS_UTF8_STARTBYTE(c) && IS_UTF8_BYTE(src[1]) && d + 2 < end) {
            // UTF-8 multi-byte encoded character

            // find how big this character is (2-4 bytes)
            size_t utf_character_size = 2;
            while(utf_character_size < 4 && src[utf_character_size] && IS_UTF8_BYTE(src[utf_character_size]) && !IS_UTF8_STARTBYTE(src[utf_character_size]))
                utf_character_size++;

            if(utf) {
                while(utf_character_size) {
                    utf_character_size--;
                    *d++ = *src++;
                }
            }
            else {
                // UTF-8 characters are not allowed.
                // Assume it is an underscore
                // and skip all except the first byte
                *d++ = '_';
                src += (utf_character_size - 1);
            }

            last_is_space = 0;
            mblen++;
            continue;
        }

        if(label_spaces_char_map[c]) {
            // a space character

            if(!last_is_space) {
                // add one space
                *d++ = char_map[c];
                mblen++;
            }

            last_is_space++;
        }
        else {
            *d++ = char_map[c];
            last_is_space = 0;
            mblen++;
        }

        src++;
    }

    // remove the last trailing space
    if(last_is_space && d > dst) {
        d--;
        mblen--;
    }

    // put a termination at the end of what we copied
    *d = '\0';

    // check if dst is all underscores and empty it if it is
    if(*dst == '_') {
        unsigned char *t = dst;
        while (*t == '_') t++;
        if (unlikely(*t == '\0')) {
            *dst = '\0';
            mblen = 0;
        }
    }

    if(unlikely(*dst == '\0')) {
        strncpyz((char *)dst, empty, dst_size);
        dst[dst_size - 1] = '\0';
        mblen = strlen((char *)dst);
        if(multibyte_length) *multibyte_length = mblen;
        return mblen;
    }

    if(multibyte_length) *multibyte_length = mblen;

    return d - dst;
}

static inline size_t rrdlabels_sanitize_name(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_names_char_map, 0, "", NULL);
}

static inline size_t rrdlabels_sanitize_value(char *dst, const char *src, size_t dst_size) {
    return text_sanitize((unsigned char *)dst, (const unsigned char *)src, dst_size, label_values_char_map, 1, "[none]", NULL);
}

// ----------------------------------------------------------------------------
// rrdlabels_create()

typedef struct rrdlabel {
    STRING *label_value;
    RRDLABEL_SRC label_source;
} RRDLABEL;

static void rrdlabel_insert_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *dict_ptr __maybe_unused) {
    RRDLABEL *lb = (RRDLABEL *)value;

    // label_value is already allocated by the STRING
    lb->label_source |=  RRDLABEL_FLAG_NEW;
    lb->label_source &= ~RRDLABEL_FLAG_OLD;
}

static void rrdlabel_delete_callback(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *dict_ptr __maybe_unused) {
    RRDLABEL *lb = (RRDLABEL *)value;

    string_freez(lb->label_value);
    lb->label_value = NULL;
}

static bool rrdlabel_conflict_callback(const DICTIONARY_ITEM *item __maybe_unused, void *oldvalue, void *newvalue, void *dict_ptr __maybe_unused) {
    RRDLABEL *lbold = (RRDLABEL *)oldvalue;
    RRDLABEL *lbnew = (RRDLABEL *)newvalue;

    if(lbold->label_value == lbnew->label_value) {
        // they are the same

        lbold->label_source |=  lbnew->label_source;
        lbold->label_source |=  RRDLABEL_FLAG_OLD;
        lbold->label_source &= ~RRDLABEL_FLAG_NEW;

        // free the new one
        string_freez(lbnew->label_value);

        return false;
    }

    // they are different

    string_freez(lbold->label_value);
    lbold->label_value  =   lbnew->label_value;
    lbold->label_source =   lbnew->label_source;
    lbold->label_source |=  RRDLABEL_FLAG_NEW;
    lbold->label_source &= ~RRDLABEL_FLAG_OLD;

    return true;
}

DICTIONARY *rrdlabels_create(void) {
    DICTIONARY *dict = dictionary_create_advanced(DICT_OPTION_DONT_OVERWRITE_VALUE | DICT_OPTION_FIXED_SIZE,
                                                  &dictionary_stats_category_rrdlabels, sizeof(RRDLABEL));

    dictionary_register_insert_callback(dict, rrdlabel_insert_callback, dict);
    dictionary_register_delete_callback(dict, rrdlabel_delete_callback, dict);
    dictionary_register_conflict_callback(dict, rrdlabel_conflict_callback, dict);
    return dict;
}


// ----------------------------------------------------------------------------
// rrdlabels_destroy()

void rrdlabels_destroy(DICTIONARY *labels_dict) {
    dictionary_destroy(labels_dict);
}

void rrdlabels_flush(DICTIONARY *labels_dict) {
    dictionary_flush(labels_dict);
}

// ----------------------------------------------------------------------------
// rrdlabels_add()

static void labels_add_already_sanitized(DICTIONARY *dict, const char *key, const char *value, RRDLABEL_SRC ls) {
    if(ls & RRDLABEL_FLAG_NEW) ls &= ~RRDLABEL_FLAG_NEW;
    if(ls & RRDLABEL_FLAG_OLD) ls &= ~RRDLABEL_FLAG_OLD;

    RRDLABEL tmp = {
        .label_source = ls,
        .label_value = string_strdupz(value)
    };
    dictionary_set(dict, key, &tmp, sizeof(RRDLABEL));
}


void rrdlabels_add(DICTIONARY *dict, const char *name, const char *value, RRDLABEL_SRC ls) {
    if(!dict) {
        error("%s(): called with NULL dictionary.", __FUNCTION__ );
        return;
    }

    char n[RRDLABELS_MAX_NAME_LENGTH + 1], v[RRDLABELS_MAX_VALUE_LENGTH + 1];
    rrdlabels_sanitize_name(n, name, RRDLABELS_MAX_NAME_LENGTH);
    rrdlabels_sanitize_value(v, value, RRDLABELS_MAX_VALUE_LENGTH);

    if(!*n) {
        error("%s: cannot add name '%s' (value '%s') which is sanitized as empty string", __FUNCTION__, name, value);
        return;
    }

    labels_add_already_sanitized(dict, n, v, ls);
}

static const char *get_quoted_string_up_to(char *dst, size_t dst_size, const char *string, char upto1, char upto2) {
    size_t len = 0;
    char *d = dst, quote = 0;
    while(*string && len++ < dst_size) {
        if(unlikely(!quote && (*string == '\'' || *string == '"'))) {
            quote = *string++;
            continue;
        }

        if(unlikely(quote && *string == quote)) {
            quote = 0;
            string++;
            continue;
        }

        if(unlikely(quote && *string == '\\' && string[1])) {
            string++;
            *d++ = *string++;
            continue;
        }

        if(unlikely(!quote && (*string == upto1 || *string == upto2))) break;

        *d++ = *string++;
    }
    *d = '\0';

    if(*string) string++;

    return string;
}

void rrdlabels_add_pair(DICTIONARY *dict, const char *string, RRDLABEL_SRC ls) {
    if(!dict) {
        error("%s(): called with NULL dictionary.", __FUNCTION__ );
        return;
    }

    char name[RRDLABELS_MAX_NAME_LENGTH + 1];
    string = get_quoted_string_up_to(name, RRDLABELS_MAX_NAME_LENGTH, string, '=', ':');

    char value[RRDLABELS_MAX_VALUE_LENGTH + 1];
    get_quoted_string_up_to(value, RRDLABELS_MAX_VALUE_LENGTH, string, '\0', '\0');

    rrdlabels_add(dict, name, value, ls);
}

// ----------------------------------------------------------------------------
// rrdlabels_get_value_to_buffer_or_null()

void rrdlabels_get_value_to_buffer_or_null(DICTIONARY *labels, BUFFER *wb, const char *key, const char *quote, const char *null) {
    if(!labels) return;

    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    if(lb && lb->label_value)
        buffer_sprintf(wb, "%s%s%s", quote, string2str(lb->label_value), quote);
    else
        buffer_strcat(wb, null);

    dictionary_acquired_item_release(labels, acquired_item);
}

void rrdlabels_value_to_buffer_array_item_or_null(DICTIONARY *labels, BUFFER *wb, const char *key) {
    if(!labels) return;

    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    if(lb && lb->label_value)
        buffer_json_add_array_item_string(wb, string2str(lb->label_value));
    else
        buffer_json_add_array_item_string(wb, NULL);

    dictionary_acquired_item_release(labels, acquired_item);
}

// ----------------------------------------------------------------------------
// rrdlabels_get_value_to_char_or_null()

void rrdlabels_get_value_strdup_or_null(DICTIONARY *labels, char **value, const char *key) {
    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    *value = (lb && lb->label_value) ? strdupz(string2str(lb->label_value)) : NULL;

    dictionary_acquired_item_release(labels, acquired_item);
}

void rrdlabels_get_value_strcpyz(DICTIONARY *labels, char *dst, size_t dst_len, const char *key) {
    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    if(lb && lb->label_value)
        strncpyz(dst, string2str(lb->label_value), dst_len);
    else
        dst[0] = '\0';

    dictionary_acquired_item_release(labels, acquired_item);
}

STRING *rrdlabels_get_value_string_dup(DICTIONARY *labels, const char *key) {
    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    STRING *ret = NULL;
    if(lb && lb->label_value)
        ret = string_dup(lb->label_value);

    dictionary_acquired_item_release(labels, acquired_item);

    return ret;
}

STRING *rrdlabels_get_value_to_buffer_or_unset(DICTIONARY *labels, BUFFER *wb, const char *key, const char *unset) {
    const DICTIONARY_ITEM *acquired_item = dictionary_get_and_acquire_item(labels, key);
    RRDLABEL *lb = dictionary_acquired_item_value(acquired_item);

    STRING *ret = NULL;
    if(lb && lb->label_value)
        buffer_strcat(wb, string2str(lb->label_value));
    else
        buffer_strcat(wb, unset);

    dictionary_acquired_item_release(labels, acquired_item);

    return ret;
}

// ----------------------------------------------------------------------------
// rrdlabels_unmark_all()
// remove labels RRDLABEL_FLAG_OLD and RRDLABEL_FLAG_NEW from all dictionary items

static int remove_flags_old_new(const DICTIONARY_ITEM *item __maybe_unused, void *value, void *data __maybe_unused) {
    RRDLABEL *lb = (RRDLABEL *)value;

    if(lb->label_source & RRDLABEL_FLAG_OLD) lb->label_source &= ~RRDLABEL_FLAG_OLD;
    if(lb->label_source & RRDLABEL_FLAG_NEW) lb->label_source &= ~RRDLABEL_FLAG_NEW;

    return 1;
}

void rrdlabels_unmark_all(DICTIONARY *labels) {
    dictionary_walkthrough_read(labels, remove_flags_old_new, NULL);
}


// ----------------------------------------------------------------------------
// rrdlabels_remove_all_unmarked()
// remove dictionary items that are neither old, nor new

static int remove_not_old_not_new_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    DICTIONARY *dict = (DICTIONARY *)data;
    RRDLABEL *lb = (RRDLABEL *)value;

    if(!(lb->label_source & (RRDLABEL_FLAG_OLD | RRDLABEL_FLAG_NEW | RRDLABEL_FLAG_PERMANENT))) {
        dictionary_del(dict, name);
        return 1;
    }

    return 0;
}

void rrdlabels_remove_all_unmarked(DICTIONARY *labels) {
    dictionary_walkthrough_write(labels, remove_not_old_not_new_callback, labels);
}


// ----------------------------------------------------------------------------
// rrdlabels_walkthrough_read()

struct labels_walkthrough {
    int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data);
    void *data;
};

static int labels_walkthrough_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    struct labels_walkthrough *d = (struct labels_walkthrough *)data;
    RRDLABEL *lb = (RRDLABEL *)value;

    RRDLABEL_SRC ls = lb->label_source;
    if(ls & RRDLABEL_FLAG_NEW) ls &= ~RRDLABEL_FLAG_NEW;
    if(ls & RRDLABEL_FLAG_OLD) ls &= ~RRDLABEL_FLAG_OLD;

    return d->callback(name, string2str(lb->label_value), ls, d->data);
}

int rrdlabels_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data) {
    struct labels_walkthrough d = {
        .callback = callback,
        .data = data
    };
    return dictionary_walkthrough_read(labels, labels_walkthrough_callback, &d);
}

int rrdlabels_sorted_walkthrough_read(DICTIONARY *labels, int (*callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *data) {
    struct labels_walkthrough d = {
        .callback = callback,
        .data = data
    };
    return dictionary_sorted_walkthrough_read(labels, labels_walkthrough_callback, &d);
}


// ----------------------------------------------------------------------------
// rrdlabels_migrate_to_these()
// migrate an existing label list to a new list, INPLACE

static int copy_label_to_dictionary_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    DICTIONARY *dst = (DICTIONARY *)data;
    RRDLABEL *lb = (RRDLABEL *)value;
    labels_add_already_sanitized(dst, name, string2str(lb->label_value), lb->label_source);
    return 1;
}

void rrdlabels_migrate_to_these(DICTIONARY *dst, DICTIONARY *src) {
    if(!dst || !src) return;

    // remove the RRDLABEL_FLAG_OLD and RRDLABEL_FLAG_NEW from all items
    rrdlabels_unmark_all(dst);

    // Mark the existing ones as RRDLABEL_FLAG_OLD,
    // or the newly added ones as RRDLABEL_FLAG_NEW
    dictionary_walkthrough_read(src, copy_label_to_dictionary_callback, dst);

    // remove the unmarked dst
    rrdlabels_remove_all_unmarked(dst);
}

void rrdlabels_copy(DICTIONARY *dst, DICTIONARY *src) {
    if(!dst || !src) return;

    dictionary_walkthrough_read(src, copy_label_to_dictionary_callback, dst);
}


// ----------------------------------------------------------------------------
// rrdlabels_match_simple_pattern()
// returns true when there are keys in the dictionary matching a simple pattern

struct simple_pattern_match_name_value {
    size_t searches;
    SIMPLE_PATTERN *pattern;
    char equal;
};

static int simple_pattern_match_name_only_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    struct simple_pattern_match_name_value *t = (struct simple_pattern_match_name_value *)data;
    (void)value;

    // we return -1 to stop the walkthrough on first match
    t->searches++;
    if(simple_pattern_matches(t->pattern, name)) return -1;

    return 0;
}

static int simple_pattern_match_name_and_value_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    struct simple_pattern_match_name_value *t = (struct simple_pattern_match_name_value *)data;
    RRDLABEL *lb = (RRDLABEL *)value;

    // we return -1 to stop the walkthrough on first match
    t->searches++;
    if(simple_pattern_matches(t->pattern, name)) return -1;

    size_t len = RRDLABELS_MAX_NAME_LENGTH + RRDLABELS_MAX_VALUE_LENGTH + 2; // +1 for =, +1 for \0
    char tmp[len], *dst = &tmp[0];
    const char *v = string2str(lb->label_value);

    // copy the name
    while(*name) *dst++ = *name++;

    // add the equal
    *dst++ = t->equal;

    // add the value
    while(*v) *dst++ = *v++;

    // terminate it
    *dst = '\0';

    t->searches++;
    if(simple_pattern_matches_length_extract(t->pattern, tmp, dst - tmp, NULL, 0) == SP_MATCHED_POSITIVE)
        return -1;

    return 0;
}

bool rrdlabels_match_simple_pattern_parsed(DICTIONARY *labels, SIMPLE_PATTERN *pattern, char equal, size_t *searches) {
    if (!labels) return false;

    struct simple_pattern_match_name_value t = {
        .searches = 0,
        .pattern = pattern,
        .equal = equal
    };

    int ret = dictionary_walkthrough_read(labels, equal?simple_pattern_match_name_and_value_callback:simple_pattern_match_name_only_callback, &t);

    if(searches)
        *searches = t.searches;

    return (ret == -1)?true:false;
}

bool rrdlabels_match_simple_pattern(DICTIONARY *labels, const char *simple_pattern_txt) {
    if (!labels) return false;

    SIMPLE_PATTERN *pattern = simple_pattern_create(simple_pattern_txt, " ,|\t\r\n\f\v", SIMPLE_PATTERN_EXACT, true);
    char equal = '\0';

    const char *s;
    for(s = simple_pattern_txt; *s ; s++) {
        if (*s == '=' || *s == ':') {
            equal = *s;
            break;
        }
    }

    bool ret = rrdlabels_match_simple_pattern_parsed(labels, pattern, equal, NULL);

    simple_pattern_free(pattern);

    return ret;
}


// ----------------------------------------------------------------------------
// Log all labels

static int rrdlabels_log_label_to_buffer_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);

    BUFFER *wb = (BUFFER *)data;
    RRDLABEL *lb = (RRDLABEL *)value;

    buffer_sprintf(wb, "Label: %s: \"%s\" (", name, string2str(lb->label_value));

    size_t sources = 0;
    if(lb->label_source & RRDLABEL_SRC_AUTO) {
        buffer_sprintf(wb, "auto");
        sources++;
    }

    if(lb->label_source & RRDLABEL_SRC_CONFIG)
        buffer_sprintf(wb, "%snetdata.conf", sources++?",":"");

    if(lb->label_source & RRDLABEL_SRC_K8S)
        buffer_sprintf(wb, "%sk8s", sources++?",":"");

    if(lb->label_source & RRDLABEL_SRC_ACLK)
        buffer_sprintf(wb, "%saclk", sources++?",":"");

    if(!sources)
        buffer_strcat(wb, "unknown");

    buffer_strcat(wb, ")\n");

    return 1;
}

void rrdlabels_log_to_buffer(DICTIONARY *labels, BUFFER *wb) {
    dictionary_sorted_walkthrough_read(labels, rrdlabels_log_label_to_buffer_callback, wb);
}


// ----------------------------------------------------------------------------
// rrdlabels_to_buffer()

struct labels_to_buffer {
    BUFFER *wb;
    bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data);
    void *filter_data;
    void (*name_sanitizer)(char *dst, const char *src, size_t dst_size);
    void (*value_sanitizer)(char *dst, const char *src, size_t dst_size);
    const char *before_each;
    const char *quote;
    const char *equal;
    const char *between_them;
    size_t count;
};

static int label_to_buffer_callback(const DICTIONARY_ITEM *item, void *value, void *data) {
    const char *name = dictionary_acquired_item_name(item);
    struct labels_to_buffer *t = (struct labels_to_buffer *)data;
    RRDLABEL *lb = (RRDLABEL *)value;

    size_t n_size = (t->name_sanitizer ) ? ( RRDLABELS_MAX_NAME_LENGTH  * 2 ) : 1;
    size_t v_size = (t->value_sanitizer) ? ( RRDLABELS_MAX_VALUE_LENGTH * 2 ) : 1;

    char n[n_size];
    char v[v_size];

    const char *nn = name, *vv = string2str(lb->label_value);

    if(t->name_sanitizer) {
        t->name_sanitizer(n, name, n_size);
        nn = n;
    }

    if(t->value_sanitizer) {
        t->value_sanitizer(v, string2str(lb->label_value), v_size);
        vv = v;
    }

    if(!t->filter_callback || t->filter_callback(name, string2str(lb->label_value), lb->label_source, t->filter_data)) {
        buffer_sprintf(t->wb, "%s%s%s%s%s%s%s%s%s", t->count++?t->between_them:"", t->before_each, t->quote, nn, t->quote, t->equal, t->quote, vv, t->quote);
        return 1;
    }

    return 0;
}

int rrdlabels_to_buffer(DICTIONARY *labels, BUFFER *wb, const char *before_each, const char *equal, const char *quote, const char *between_them, bool (*filter_callback)(const char *name, const char *value, RRDLABEL_SRC ls, void *data), void *filter_data, void (*name_sanitizer)(char *dst, const char *src, size_t dst_size), void (*value_sanitizer)(char *dst, const char *src, size_t dst_size)) {
    struct labels_to_buffer tmp = {
        .wb = wb,
        .filter_callback = filter_callback,
        .filter_data = filter_data,
        .name_sanitizer = name_sanitizer,
        .value_sanitizer = value_sanitizer,
        .before_each = before_each,
        .equal = equal,
        .quote = quote,
        .between_them = between_them,
        .count = 0
    };
    return dictionary_walkthrough_read(labels, label_to_buffer_callback, (void *)&tmp);
}

void rrdlabels_to_buffer_json_members(DICTIONARY *labels, BUFFER *wb) {
    RRDLABEL *lb;
    dfe_start_read(labels, lb) {
        buffer_json_member_add_string(wb, lb_dfe.name, string2str(lb->label_value));
    }
    dfe_done(lb);
}

void rrdset_update_rrdlabels(RRDSET *st, DICTIONARY *new_rrdlabels) {
    if(!st->rrdlabels)
        st->rrdlabels = rrdlabels_create();

    if (new_rrdlabels)
        rrdlabels_migrate_to_these(st->rrdlabels, new_rrdlabels);

    rrdset_flag_set(st, RRDSET_FLAG_METADATA_UPDATE);
    rrdhost_flag_set(st->rrdhost, RRDHOST_FLAG_METADATA_UPDATE);
}


// ----------------------------------------------------------------------------
// rrdlabels unit test

struct rrdlabels_unittest_add_a_pair {
    const char *pair;
    const char *expected_name;
    const char *expected_value;
    const char *name;
    const char *value;
    RRDLABEL_SRC ls;
    int errors;
};

int rrdlabels_unittest_add_a_pair_callback(const char *name, const char *value, RRDLABEL_SRC ls, void *data) {
    struct rrdlabels_unittest_add_a_pair *t = (struct rrdlabels_unittest_add_a_pair *)data;

    t->name = name;
    t->value = value;
    t->ls = ls;

    if(strcmp(name, t->expected_name) != 0) {
        fprintf(stderr, "name is wrong, found \"%s\", expected \"%s\"", name, t->expected_name);
        t->errors++;
    }

    if(value == NULL && t->expected_value == NULL) {
        ;
    }
    else if(value == NULL || t->expected_value == NULL) {
        fprintf(stderr, "value is wrong, found \"%s\", expected \"%s\"", value?value:"(null)", t->expected_value?t->expected_value:"(null)");
        t->errors++;
    }
    else if(strcmp(value, t->expected_value) != 0) {
        fprintf(stderr, "values don't match, found \"%s\", expected \"%s\"", value, t->expected_value);
        t->errors++;
    }

    return 1;
}

int rrdlabels_unittest_add_a_pair(const char *pair, const char *name, const char *value) {
    DICTIONARY *labels = rrdlabels_create();
    int errors;

    fprintf(stderr, "rrdlabels_add_pair(labels, %s) ... ", pair);

    rrdlabels_add_pair(labels, pair, RRDLABEL_SRC_CONFIG);

    struct rrdlabels_unittest_add_a_pair tmp = {
        .pair = pair,
        .expected_name = name,
        .expected_value = value,
        .errors = 0
    };
    int ret = rrdlabels_walkthrough_read(labels, rrdlabels_unittest_add_a_pair_callback, &tmp);
    errors = tmp.errors;
    if(ret != 1) {
        fprintf(stderr, "failed to get \"%s\" label", name);
        errors++;
    }

    if(!errors)
        fprintf(stderr, " OK, name='%s' and value='%s'\n", tmp.name, tmp.value?tmp.value:"(null)");
    else
        fprintf(stderr, " FAILED\n");

    rrdlabels_destroy(labels);
    return errors;
}

int rrdlabels_unittest_add_pairs() {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int errors = 0;

    // basic test
    errors += rrdlabels_unittest_add_a_pair("tag=value", "tag", "value");
    errors += rrdlabels_unittest_add_a_pair("tag:value", "tag", "value");

    // test newlines
    errors += rrdlabels_unittest_add_a_pair("   tag   = \t value \r\n", "tag", "value");

    // test : in values
    errors += rrdlabels_unittest_add_a_pair("tag=:value", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("tag::value", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("   tag   =   :value ", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("   tag   :   :value ", "tag", ":value");
    errors += rrdlabels_unittest_add_a_pair("tag:5", "tag", "5");
    errors += rrdlabels_unittest_add_a_pair("tag:55", "tag", "55");
    errors += rrdlabels_unittest_add_a_pair("tag:aa", "tag", "aa");
    errors += rrdlabels_unittest_add_a_pair("tag:a", "tag", "a");

    // test empty values
    errors += rrdlabels_unittest_add_a_pair("tag", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:\"\"", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:''", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag:\r\n", "tag", "[none]");
    errors += rrdlabels_unittest_add_a_pair("tag\r\n", "tag", "[none]");

    // test UTF-8 in values
    errors += rrdlabels_unittest_add_a_pair("tag: country:Ελλάδα", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag\": \"country:Ελλάδα\"", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag\": country:\"Ελλάδα\"", "tag", "country:Ελλάδα");
    errors += rrdlabels_unittest_add_a_pair("\"tag=1\": country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece");
    errors += rrdlabels_unittest_add_a_pair("\"tag=1\" = country:\"Gre\\\"ece\"", "tag_1", "country:Gre_ece");

    errors += rrdlabels_unittest_add_a_pair("\t'LABE=L'\t=\t\"World\" peace", "labe_l", "World peace");
    errors += rrdlabels_unittest_add_a_pair("\t'LA\\'B:EL'\t=\tcountry:\"World\":\"Europe\":\"Greece\"", "la_b_el", "country:World:Europe:Greece");
    errors += rrdlabels_unittest_add_a_pair("\t'LA\\'B:EL'\t=\tcountry\\\"World\"\\\"Europe\"\\\"Greece\"", "la_b_el", "country/World/Europe/Greece");

    errors += rrdlabels_unittest_add_a_pair("NAME=\"VALUE\"", "name", "VALUE");
    errors += rrdlabels_unittest_add_a_pair("\"NAME\" : \"VALUE\"", "name", "VALUE");
    errors += rrdlabels_unittest_add_a_pair("NAME: \"VALUE\"", "name", "VALUE");

    return errors;
}

int rrdlabels_unittest_check_simple_pattern(DICTIONARY *labels, const char *pattern, bool expected) {
    fprintf(stderr, "rrdlabels_match_simple_pattern(labels, \"%s\") ... ", pattern);

    bool ret = rrdlabels_match_simple_pattern(labels, pattern);
    fprintf(stderr, "%s, got %s expected %s\n", (ret == expected)?"OK":"FAILED", ret?"true":"false", expected?"true":"false");

    return (ret == expected)?0:1;
}

int rrdlabels_unittest_simple_pattern() {
    fprintf(stderr, "\n%s() tests\n", __FUNCTION__);

    int errors = 0;

    DICTIONARY *labels = rrdlabels_create();
    rrdlabels_add(labels, "tag1", "value1", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag2", "value2", RRDLABEL_SRC_CONFIG);
    rrdlabels_add(labels, "tag3", "value3", RRDLABEL_SRC_CONFIG);

    errors += rrdlabels_unittest_check_simple_pattern(labels, "*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*1", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "value*", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*=value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*:value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "*2 *3", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag3 *2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1 tag2", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1tag2", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "invalid1 invalid2 tag3", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag1 tag4", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1=value1", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag1=value2", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "tag*=value*", true);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag*=value*", false);
    errors += rrdlabels_unittest_check_simple_pattern(labels, "!tag2=something2 tag2=*2", true);

    rrdlabels_destroy(labels);

    return errors;
}

int rrdlabels_unittest_sanitize_value(const char *src, const char *expected) {
    char buf[RRDLABELS_MAX_VALUE_LENGTH + 1];
    size_t len = rrdlabels_sanitize_value(buf, src, RRDLABELS_MAX_VALUE_LENGTH);
    size_t expected_len = strlen(expected);

    int err = 0;
    if(strcmp(buf, expected) != 0) err = 1;
    if(len != expected_len) err = 1;

    fprintf(stderr, "%s(%s): %s, expected '%s', got '%s', expected bytes = %zu, got bytes = %zu\n", __FUNCTION__, src, (err==1)?"FAILED":"OK", expected, buf, expected_len, strlen(buf));
    return err;
}

int rrdlabels_unittest_sanitization() {
    int errors = 0;

    errors += rrdlabels_unittest_sanitize_value("", "[none]");
    errors += rrdlabels_unittest_sanitize_value("1", "1");
    errors += rrdlabels_unittest_sanitize_value("  hello   world   ", "hello world");
    errors += rrdlabels_unittest_sanitize_value("[none]", "[none]");

    // 2-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value(" Ελλάδα ", "Ελλάδα");
    errors += rrdlabels_unittest_sanitize_value("aŰbŲcŴ", "aŰbŲcŴ");
    errors += rrdlabels_unittest_sanitize_value("Ű b Ų c Ŵ", "Ű b Ų c Ŵ");

    // 3-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value("‱", "‱");
    errors += rrdlabels_unittest_sanitize_value("a‱b", "a‱b");
    errors += rrdlabels_unittest_sanitize_value("a ‱ b", "a ‱ b");

    // 4-byte UTF-8
    errors += rrdlabels_unittest_sanitize_value("𩸽", "𩸽");
    errors += rrdlabels_unittest_sanitize_value("a𩸽b", "a𩸽b");
    errors += rrdlabels_unittest_sanitize_value("a 𩸽 b", "a 𩸽 b");

    // mixed multi-byte
    errors += rrdlabels_unittest_sanitize_value("Ű‱𩸽‱Ű", "Ű‱𩸽‱Ű");

    return errors;
}

int rrdlabels_unittest(void) {
    int errors = 0;

    errors += rrdlabels_unittest_sanitization();
    errors += rrdlabels_unittest_add_pairs();
    errors += rrdlabels_unittest_simple_pattern();

    fprintf(stderr, "%d errors found\n", errors);
    return errors;
}
