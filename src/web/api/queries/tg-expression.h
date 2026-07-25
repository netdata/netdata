// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_API_QUERY_TG_EXPRESSION_H
#define NETDATA_API_QUERY_TG_EXPRESSION_H

#include "query.h"

// The condition shared by the fleet time-aggregations:
//
//   percentage-of-samples (a.k.a. countif)   number-of-flaps
//   percentage-of-time                       number-of-times
//
// and by the health `lookup:` line, so the API and alerts cannot drift
// apart again - they used to disagree on a bare number and on `<:`/`>:`.
//
// The grammar is an operator plus one operand:
//
//   operator  >  >=|>:  <  <=|<:  =|==|:  !=|!|<>
//   operand   a number, a gap token (nan|null|gap|empty), or the
//             predecessor keyword (previous|last)
//
// There are deliberately no and/or compounds: a single comparison keeps
// the feature explainable, and gaps are reachable as a VALUE instead.

typedef enum tg_expression_cmp {
    TG_EXPRESSION_EQUAL,
    TG_EXPRESSION_NOTEQUAL,
    TG_EXPRESSION_LESS,
    TG_EXPRESSION_LESSEQUAL,
    TG_EXPRESSION_GREATER,
    TG_EXPRESSION_GREATEREQUAL,
} TG_EXPRESSION_CMP;

typedef enum tg_expression_operand {
    TG_EXPRESSION_OPERAND_NUMBER = 0,   // compare against a constant
    TG_EXPRESSION_OPERAND_GAP,          // compare against "no data"
    TG_EXPRESSION_OPERAND_PREVIOUS,     // compare against the last collected sample
} TG_EXPRESSION_OPERAND;

typedef struct tg_expression {
    TG_EXPRESSION_CMP cmp;
    TG_EXPRESSION_OPERAND operand;
    NETDATA_DOUBLE target;              // TG_EXPRESSION_OPERAND_NUMBER only

    // the predecessor, carried across buckets and cleared per dimension
    NETDATA_DOUBLE previous;
    bool has_previous;
} TG_EXPRESSION;

static inline bool tg_expression_token(const char *s, const char *token, size_t len) {
    // a token must be followed by the end of the string or whitespace, so
    // "lasting" is not read as "last"
    return strncasecmp(s, token, len) == 0 && (s[len] == '\0' || isspace((uint8_t)s[len]));
}

// Returns false when the operand is malformed. The query API has always
// been lenient here (an unparsable condition silently compares equal to
// zero) and stays that way; health checks the result and refuses to load
// an alert whose condition it cannot read, which is what it has always
// done.
static inline bool tg_expression_parse(TG_EXPRESSION *e, const char *options) {
    e->cmp = TG_EXPRESSION_EQUAL;
    e->operand = TG_EXPRESSION_OPERAND_NUMBER;
    e->target = 0.0;
    e->previous = 0.0;
    e->has_previous = false;

    if(!options || !*options)
        return true;

    const char *s = options;
    while(isspace((uint8_t)*s)) s++;

    switch(*s) {
        case '!':
            s++;
            if(*s == '=' || *s == ':') s++;
            e->cmp = TG_EXPRESSION_NOTEQUAL;
            break;

        case '>':
            s++;
            if(*s == '=' || *s == ':') {
                s++;
                e->cmp = TG_EXPRESSION_GREATEREQUAL;
            }
            else
                e->cmp = TG_EXPRESSION_GREATER;
            break;

        case '<':
            s++;
            if(*s == '>') {
                s++;
                e->cmp = TG_EXPRESSION_NOTEQUAL;
            }
            else if(*s == '=' || *s == ':') {
                s++;
                e->cmp = TG_EXPRESSION_LESSEQUAL;
            }
            else
                e->cmp = TG_EXPRESSION_LESS;
            break;

        case '=':
            s++;
            if(*s == '=') s++;
            e->cmp = TG_EXPRESSION_EQUAL;
            break;

        case ':':
            s++;
            e->cmp = TG_EXPRESSION_EQUAL;
            break;

        default:
            // no operator at all: the operand stands alone and compares
            // EQUAL. tg_countif_create used to advance one character here
            // regardless, swallowing the first digit of a bare number, so
            // "5" targeted 0. Health never had that bug; this is the side
            // that moves.
            e->cmp = TG_EXPRESSION_EQUAL;
            break;
    }

    while(isspace((uint8_t)*s)) s++;

    if(!*s)
        return true;

    if(isalpha((uint8_t)*s)) {
        if(tg_expression_token(s, "gap", 3) ||
           tg_expression_token(s, "nan", 3) ||
           tg_expression_token(s, "null", 4) ||
           tg_expression_token(s, "empty", 5))
            e->operand = TG_EXPRESSION_OPERAND_GAP;

        else if(tg_expression_token(s, "previous", 8) ||
                tg_expression_token(s, "last", 4))
            e->operand = TG_EXPRESSION_OPERAND_PREVIOUS;

        else
            return false;   // a word we do not know

        return true;
    }

    // the operand must LOOK like a number before we trust str2ndd with it,
    // so a lone '.', a stray operator or trailing junk is rejected instead
    // of silently becoming zero
    if(!(isdigit((uint8_t)*s) ||
         (*s == '.' && isdigit((uint8_t)s[1])) ||
         ((*s == '-' || *s == '+') && s[1] &&
          (isdigit((uint8_t)s[1]) || (s[1] == '.' && isdigit((uint8_t)s[2]))))))
        return false;

    char *end = NULL;
    e->target = str2ndd(s, &end);

    if(!end || end == s)
        return false;

    while(isspace((uint8_t)*end)) end++;
    if(*end)
        return false;       // trailing junk

    return true;
}

// true when the expression names a gap token, which is the ONLY way gap
// slots reach a grouping - otherwise they stay invisible, exactly as they
// are for every other time-aggregation
static inline bool tg_expression_wants_gaps(const TG_EXPRESSION *e) {
    return e->operand == TG_EXPRESSION_OPERAND_GAP;
}

// cleared when the query engine switches dimensions: the predecessor
// belongs to one series
static inline void tg_expression_reset(TG_EXPRESSION *e) {
    e->previous = 0.0;
    e->has_previous = false;
}

static inline bool tg_expression_compare(TG_EXPRESSION_CMP cmp, NETDATA_DOUBLE value, NETDATA_DOUBLE target) {
    switch(cmp) {
        case TG_EXPRESSION_GREATER:      return value >  target;
        case TG_EXPRESSION_GREATEREQUAL: return value >= target;
        case TG_EXPRESSION_LESS:         return value <  target;
        case TG_EXPRESSION_LESSEQUAL:    return value <= target;
        case TG_EXPRESSION_EQUAL:        return value == target;
        case TG_EXPRESSION_NOTEQUAL:     return value != target;
    }
    return false;
}

// Evaluates one delivered point and advances the predecessor.
//
// The NaN model: only equality is defined against a gap, so every ordered
// comparison involving one is false, and a numeric comparison on a gap
// slot is false too. The predecessor advances on collected samples ONLY,
// so a counter drop across a gap is still a drop.
static inline bool tg_expression_eval(TG_EXPRESSION *e, NETDATA_DOUBLE value, bool is_gap) {
    bool matched = false;

    if(e->operand == TG_EXPRESSION_OPERAND_GAP) {
        if(e->cmp == TG_EXPRESSION_EQUAL)
            matched = is_gap;
        else if(e->cmp == TG_EXPRESSION_NOTEQUAL)
            matched = !is_gap;
        else
            matched = false;
    }
    else if(is_gap)
        matched = false;

    else if(e->operand == TG_EXPRESSION_OPERAND_PREVIOUS) {
        // the first collected sample of a query has no predecessor and
        // can never match
        if(likely(e->has_previous))
            matched = tg_expression_compare(e->cmp, value, e->previous);
    }
    else
        matched = tg_expression_compare(e->cmp, value, e->target);

    if(!is_gap) {
        e->previous = value;
        e->has_previous = true;
    }

    return matched;
}

#endif //NETDATA_API_QUERY_TG_EXPRESSION_H
