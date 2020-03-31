#include <stdlib.h>

#include "../libnetdata.h"
#include "jsmn.h"

static inline int output_characters(char *wstr, unsigned long unicode_char)
{
    if (unicode_char < 0x80) {
        *wstr = (char)((unicode_char >> 0 & 0x7F) | 0x00);
        return 1;
    } else if (unicode_char < 0x0800) {
        *wstr = (char)((unicode_char >> 6 & 0x1F) | 0xc0);
        *(wstr + 1) = (char)((unicode_char >> 0 & 0x3F) | 0x80);
        return 2;
    } else if (unicode_char < 0x010000) {
        *wstr = (char)((unicode_char >> 12 & 0x0F) | 0xE0);
        *(wstr + 1) = (char)((unicode_char >> 6 & 0x3F) | 0x80);
        *(wstr + 2) = (char)((unicode_char >> 0 & 0x3F) | 0x80);
        return 3;
    } else {
        *wstr = (char)(unicode_char >> 18 & 0x07 | 0xF0);
        *(wstr + 1) = (char)((unicode_char >> 12 & 0x3F) | 0x80);
        *(wstr + 2) = (char)((unicode_char >> 6 & 0x3F) | 0x80);
        *(wstr + 3) = (char)((unicode_char >> 0 & 0x3F) | 0x80);
        return 4;
    }
}

/**
 * Alloc token
 *
 * Allocates a fresh unused token from the token pull.
 *
 * @param parser the controller
 * @param tokens the tokens I am working
 * @param num_tokens the number total of tokens.
 *
 * @return it returns the next token to work.
 */
static jsmntok_t *jsmn_alloc_token(jsmn_parser *parser,
                                   jsmntok_t *tokens, size_t num_tokens) {
    jsmntok_t *tok;
    if (parser->toknext >= num_tokens) {
        return NULL;
    }
    tok = &tokens[parser->toknext++];
    tok->start = tok->end = -1;
    tok->size = 0;
#ifdef JSMN_PARENT_LINKS
    tok->parent = -1;
#endif
    return tok;
}

/**
 * Fill Token
 *
 * Fills token type and boundaries.
 *
 * @param token the structure to set the values
 * @param type is the token type
 * @param start is the first position of the value
 * @param end is the end of the value
 */
static void jsmn_fill_token(jsmntok_t *token, jsmntype_t type,
                            int start, int end) {
    token->type = type;
    token->start = start;
    token->end = end;
    token->size = 0;
}

/**
 * Parse primitive
 *
 * Fills next available token with JSON primitive.
 *
 * @param parser is the control structure
 * @param js is the json string
 * @param type is the token type
 */
static jsmnerr_t jsmn_parse_primitive(jsmn_parser *parser, const char *js,
                                      size_t len, jsmntok_t *tokens, size_t num_tokens) {
    jsmntok_t *token;
    int start;

    start = parser->pos;

    for (; parser->pos < len && js[parser->pos] != '\0'; parser->pos++) {
        switch (js[parser->pos]) {
#ifndef JSMN_STRICT
            /* In strict mode primitive must be followed by "," or "}" or "]" */
            case ':':
#endif
            case '\t' : case '\r' : case '\n' : case ' ' :
            case ','  : case ']'  : case '}' :
                goto found;
        }
        if (js[parser->pos] < 32 || js[parser->pos] >= 127) {
            parser->pos = start;
            return JSMN_ERROR_INVAL;
        }
    }
#ifdef JSMN_STRICT
    /* In strict mode primitive must be followed by a comma/object/array */
	parser->pos = start;
	return JSMN_ERROR_PART;
#endif

    found:
    if (tokens == NULL) {
        parser->pos--;
        return 0;
    }
    token = jsmn_alloc_token(parser, tokens, num_tokens);
    if (token == NULL) {
        parser->pos = start;
        return JSMN_ERROR_NOMEM;
    }
    jsmn_fill_token(token, JSMN_PRIMITIVE, start, parser->pos);
#ifdef JSMN_PARENT_LINKS
    token->parent = parser->toksuper;
#endif
    parser->pos--;
    return 0;
}

/**
 * Parse string
 *
 * Fills next token with JSON string.
 *
 * @param parser is the control structure
 * @param js is the json string
 * @param len is the js length
 * @param tokens is structure with the tokens mapped.
 * @param num_tokens is the total number of tokens
 *
 * @return It returns 0 on success and another integer otherwise
 */
static jsmnerr_t jsmn_parse_string(jsmn_parser *parser, char *js,
                                   size_t len, jsmntok_t *tokens, size_t num_tokens) {
    jsmntok_t *token;

    int start = parser->pos;

    parser->pos++;

    /* Skip starting quote */
    for (; parser->pos < len && js[parser->pos] != '\0'; parser->pos++) {
        char c = js[parser->pos];

        /* Quote: end of string */
        if (c == '\"') {
            if (tokens == NULL) {
                return 0;
            }
            token = jsmn_alloc_token(parser, tokens, num_tokens);
            if (token == NULL) {
                parser->pos = start;
                return JSMN_ERROR_NOMEM;
            }
            jsmn_fill_token(token, JSMN_STRING, start+1, parser->pos);
#ifdef JSMN_PARENT_LINKS
            token->parent = parser->toksuper;
#endif
            return 0;
        }

        /* Backslash: Quoted symbol expected */
        if (c == '\\') {
            unsigned long unicode_char = 0;
            parser->pos++;
            switch (js[parser->pos]) {
                /* Allowed escaped symbols */
                case '\"':
                case '/':
                case '\\':
                    js[parser->pos-1] = js[parser->pos];
                    break;
                case 'b':
                    js[parser->pos-1] = '\b';
                    break;
                case 'f':
                    js[parser->pos-1]  = '\f';
                    break;
                case 'r':
                    js[parser->pos-1]  = '\r';
                    break;
                case 'n':
                    js[parser->pos-1]  = '\n';
                    break;
                case 't':
                    js[parser->pos-1]  = '\t';
                    break;
                    /* Allows escaped symbol \uXXXX */
                case 'u':
                    parser->pos++;
                    int i = 0;
                    for(; i < 4 && js[parser->pos] != '\0'; i++) {
                        /* If it isn't a hex character we have an error */
                        unicode_char = unicode_char << 4;
                        switch (js[parser->pos]) {
                            case 48 ... 57: /* 0-9 */
                                unicode_char += (js[parser->pos] - 48);
                                break;
                            case 65 ... 70: /* A-F */
                                unicode_char += (js[parser->pos] - 55);
                                break;
                            case 97 ... 102: /* a-f */
                                unicode_char += (js[parser->pos] - 87);
                                break;
                            default:
                                parser->pos = start;
                                return JSMN_ERROR_INVAL;
                        }
                        parser->pos++;
                    }
                    // The last character on unicode
                    char *tmp = &js[parser->pos-1];
                    // unicode symbol exists from parser (pos - 4, pos - 1)
                    // should be written on pos - 6
                    parser->pos = parser->pos - 6;
                    // position will be on last valid character and it will advanced
                    parser->pos = parser->pos + output_characters(&js[parser->pos], unicode_char);
                    char *dest = &js[parser->pos];
                    while (*tmp) {
                        *dest = *(tmp + 1);
                        tmp++;
                        dest++;
                    }
                    break;
                    /* Unexpected symbol */
                default:
                    parser->pos = start;
                    return JSMN_ERROR_INVAL;
            }
        }
    }
    parser->pos = start;
    return JSMN_ERROR_PART;
}

/**
 * JSMN Parse
 *
 * Parse JSON string and fill tokens.
 *
 * @param parser the auxiliar vector used to parser
 * @param js the string to parse
 * @param len the string length
 * @param tokens the place to map the tokens
 * @param num_tokens the number of tokens present in the tokens structure.
 *
 * @return It returns the number of tokens present in the string on success or a negative number otherwise
 */
jsmnerr_t jsmn_parse(jsmn_parser *parser, const char *js, size_t len,
                     jsmntok_t *tokens, unsigned int num_tokens) {
    jsmnerr_t r;
    int i;
    jsmntok_t *token;
    int count = 0;

    for (; parser->pos < len && js[parser->pos] != '\0'; parser->pos++) {
        char c;
        jsmntype_t type;

        c = js[parser->pos];
        switch (c) {
            case '{': case '[':
                count++;
                if (tokens == NULL) {
                    break;
                }
                token = jsmn_alloc_token(parser, tokens, num_tokens);
                if (token == NULL)
                    return JSMN_ERROR_NOMEM;
                if (parser->toksuper != -1) {
                    tokens[parser->toksuper].size++;
#ifdef JSMN_PARENT_LINKS
                    token->parent = parser->toksuper;
#endif
                }
                token->type = (c == '{' ? JSMN_OBJECT : JSMN_ARRAY);
                token->start = parser->pos;
                parser->toksuper = parser->toknext - 1;
                break;
            case '}': case ']':
                if (tokens == NULL)
                    break;
                type = (c == '}' ? JSMN_OBJECT : JSMN_ARRAY);
#ifdef JSMN_PARENT_LINKS
            if (parser->toknext < 1) {
					return JSMN_ERROR_INVAL;
				}
				token = &tokens[parser->toknext - 1];
				for (;;) {
					if (token->start != -1 && token->end == -1) {
						if (token->type != type) {
							return JSMN_ERROR_INVAL;
						}
						token->end = parser->pos + 1;
						parser->toksuper = token->parent;
						break;
					}
					if (token->parent == -1) {
						break;
					}
					token = &tokens[token->parent];
				}
#else
                for (i = parser->toknext - 1; i >= 0; i--) {
                    token = &tokens[i];
                    if (token->start != -1 && token->end == -1) {
                        if (token->type != type) {
                            return JSMN_ERROR_INVAL;
                        }
                        parser->toksuper = -1;
                        token->end = parser->pos + 1;
                        break;
                    }
                }
                /* Error if unmatched closing bracket */
                if (i == -1) return JSMN_ERROR_INVAL;
                for (; i >= 0; i--) {
                    token = &tokens[i];
                    if (token->start != -1 && token->end == -1) {
                        parser->toksuper = i;
                        break;
                    }
                }
#endif
                break;
            case '\"':
                r = jsmn_parse_string(parser, js, len, tokens, num_tokens);
                if (r < 0) return r;
                count++;
                if (parser->toksuper != -1 && tokens != NULL)
                    tokens[parser->toksuper].size++;
                break;
            case '\t' : case '\r' : case '\n' : case ':' : case ',': case ' ':
                break;
#ifdef JSMN_STRICT
            /* In strict mode primitives are: numbers and booleans */
			case '-': case '0': case '1' : case '2': case '3' : case '4':
			case '5': case '6': case '7' : case '8': case '9':
			case 't': case 'f': case 'n' :
#else
                /* In non-strict mode every unquoted value is a primitive */
            default:
#endif
                r = jsmn_parse_primitive(parser, js, len, tokens, num_tokens);
                if (r < 0) return r;
                count++;
                if (parser->toksuper != -1 && tokens != NULL)
                    tokens[parser->toksuper].size++;
                break;

#ifdef JSMN_STRICT
            /* Unexpected char in strict mode */
			default:
				return JSMN_ERROR_INVAL;
#endif
        }
    }

    if (tokens) {
        for (i = parser->toknext - 1; i >= 0; i--) {
            /* Unmatched opened object or array */
            if (tokens[i].start != -1 && tokens[i].end == -1) {
                return JSMN_ERROR_PART;
            }
        }
    }

    return count;
}

/**
 * JSMN Init
 *
 * Creates a new parser based over a given  buffer with an array of tokens
 * available.
 *
 * @param parser is the structure with values to reset
 */
void jsmn_init(jsmn_parser *parser) {
    parser->pos = 0;
    parser->toknext = 0;
    parser->toksuper = -1;
}