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

// the groupings whose value is derived from a condition, not from the
// sample values themselves - the only ones that carry an expression.
// percentile and the trimmed-* pair also take a parenthesised argument,
// but theirs is a plain number.
static inline bool time_grouping_is_expression(RRDR_TIME_GROUPING g) {
    switch(g) {
        case RRDR_GROUPING_COUNTIF:
        case RRDR_GROUPING_PERCENTAGE_OF_TIME:
        case RRDR_GROUPING_NUMBER_OF_FLAPS:
        case RRDR_GROUPING_NUMBER_OF_TIMES:
            return true;

        default:
            return false;
    }
}

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

    // the highest value of the previous stored window, for the
    // monotone-drop rule above tier 0
    NETDATA_DOUBLE previous_max;
} TG_EXPRESSION;

// One delivered point, as the expression groupings see it.
//
// Above tier 0 a stored point is not a sample: it is min/max/avg over
// `count` raw samples. `duration` is the slice of the current bucket this
// point covers, and `samples` the number of sample slots it stands for.
typedef struct tg_point {
    NETDATA_DOUBLE value;       // the fetched value; INTERPOLATED on a re-delivery
    NETDATA_DOUBLE sum;         // the window's own sum - interpolation never touches it
    NETDATA_DOUBLE min;
    NETDATA_DOUBLE max;
    size_t count;               // raw samples behind it; 1 at tier 0
    time_t duration;
    size_t samples;
    bool is_gap;

    // false when this is a REPEAT of a stored point already delivered to an
    // earlier bucket. A point wider than the bucket is delivered again with
    // an interpolated value, so only the first delivery may advance state or
    // count an occurrence - the time, however, is real every time.
    bool first;
} TG_POINT;

// the average of the window itself, immune to the interpolation applied to
// `value` when a wide point is re-delivered
static inline NETDATA_DOUBLE tg_point_average(const TG_POINT *p) {
    if(p->count > 1 && netdata_double_isnumber(p->sum))
        return p->sum / (NETDATA_DOUBLE)p->count;

    return p->value;
}

static inline bool tg_point_is_window(const TG_POINT *p) {
    // a stored point that aggregates more than one raw sample. A window
    // whose extremes are equal is still a window, not a sample: its value
    // is its own average, never the interpolated value the query engine
    // hands to a view bucket narrower than the window.
    return p->count > 1 && netdata_double_isnumber(p->min) &&
           netdata_double_isnumber(p->max);
}

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

        // nothing may follow it: the grammar has one operand and no
        // and/or compounds, so "gap and something" is not a condition
        while(*s && !isspace((uint8_t)*s)) s++;
        while(isspace((uint8_t)*s)) s++;
        return *s == '\0';
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
    e->previous_max = 0.0;
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

// The share of a STORED WINDOW that satisfied the condition, estimated
// from the only statistics a tier keeps: min, max and the average.
//
// The model treats the window as if the value took just two values, min
// and max, weighted so their mean is the recorded average:
//
//     weight(max) = (avg - min) / (max - min)
//
// That is EXACT for a 0/1 dimension - the shape a fleet availability
// signal has - because there the average IS the fraction of time at 1. It
// is an approximation for a continuous metric, where the interior estimate
// does not move with the threshold; tier 0 is exact, so a query that needs
// precision on a continuous threshold should ask for tier 0.
//
// Evaluating the condition on the average instead (what the engine does
// for every other aggregation) is not usable here: a minute that was up
// 30s and down 30s records avg=0.5, and both `>=1` and `==0` would then
// answer "never".
static inline NETDATA_DOUBLE tg_expression_window_fraction(
    TG_EXPRESSION *e, const TG_POINT *p) {

    if(p->max <= p->min)
        // the window never moved, so there is nothing to estimate: it held
        // one value for its whole duration and either satisfied the
        // condition or did not
        return tg_expression_compare(e->cmp, tg_point_average(p), e->target) ? 1.0 : 0.0;

    NETDATA_DOUBLE w = (tg_point_average(p) - p->min) / (p->max - p->min);
    if(w < 0.0) w = 0.0;
    if(w > 1.0) w = 1.0;

    const NETDATA_DOUBLE t = e->target;

    switch(e->cmp) {
        case TG_EXPRESSION_GREATEREQUAL:
            if(t <= p->min) return 1.0;
            if(t >  p->max) return 0.0;
            return w;

        case TG_EXPRESSION_GREATER:
            if(t <  p->min) return 1.0;
            if(t >= p->max) return 0.0;
            return w;

        case TG_EXPRESSION_LESSEQUAL:
            if(t >= p->max) return 1.0;
            if(t <  p->min) return 0.0;
            return 1.0 - w;

        case TG_EXPRESSION_LESS:
            if(t >  p->max) return 1.0;
            if(t <= p->min) return 0.0;
            return 1.0 - w;

        case TG_EXPRESSION_EQUAL:
            // only the two mass points carry any weight under this model
            if(t == p->min) return 1.0 - w;
            if(t == p->max) return w;
            return 0.0;

        case TG_EXPRESSION_NOTEQUAL:
            if(t == p->min) return w;
            if(t == p->max) return 1.0 - w;
            return 1.0;
    }

    return 0.0;
}

// The share of one delivered point that satisfied the condition: 0 or 1
// for a sample, an estimate in between for a stored window.
//
// Gap and predecessor operands are NOT estimated across a window - a
// stored window keeps no gap positions and no ordering. `<previous` is the
// exception: a window whose minimum falls below the previous window's
// maximum proves at least one drop, which is what counts a reboot.
static inline NETDATA_DOUBLE tg_expression_share(TG_EXPRESSION *e, const TG_POINT *p) {
    NETDATA_DOUBLE share;

    if(e->operand == TG_EXPRESSION_OPERAND_PREVIOUS && p->count > 1 && e->has_previous) {
        // Only the monotone drop survives a rollup: a window whose minimum
        // falls below the previous window's maximum proves the value went
        // backwards, which is what counts a reboot. Every other predecessor
        // comparison is NOT estimated across a window - it is answered on
        // the window's own average, which is documented, because a rollup
        // keeps no ordering to compare against.
        if(e->cmp == TG_EXPRESSION_LESS &&
           netdata_double_isnumber(p->min) && p->min < e->previous_max)
            share = 1.0;
        else
            share = tg_expression_eval(e, tg_point_average(p), p->is_gap) ? 1.0 : 0.0;

        if(netdata_double_isnumber(p->max))
            e->previous_max = p->max;

        return share;
    }

    if(e->operand == TG_EXPRESSION_OPERAND_NUMBER && tg_point_is_window(p)) {
        share = tg_expression_window_fraction(e, p);

        if(p->first && !p->is_gap) {
            e->previous = tg_point_average(p);
            e->has_previous = true;
            if(netdata_double_isnumber(p->max))
                e->previous_max = p->max;
        }
        return share;
    }

    if(!p->first) {
        // a repeat of a point already accounted for: answer it, but leave
        // the predecessor where the first delivery left it
        TG_EXPRESSION probe = *e;
        return tg_expression_eval(&probe, p->value, p->is_gap) ? 1.0 : 0.0;
    }

    share = tg_expression_eval(e, p->value, p->is_gap) ? 1.0 : 0.0;
    if(!p->is_gap && netdata_double_isnumber(p->max))
        e->previous_max = p->max;

    return share;
}

#endif //NETDATA_API_QUERY_TG_EXPRESSION_H
