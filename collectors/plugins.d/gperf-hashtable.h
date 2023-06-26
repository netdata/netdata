// SPDX-License-Identifier: GPL-3.0-or-later
/* ANSI-C code produced by gperf version 3.1 */
/* Command-line: gperf --multiple-iterations=1000 --hash-function-name=gperf_keyword_hash_function --lookup-function-name=gperf_lookup_keyword --word-array-name=gperf_keywords --constants-prefix=GPERF_PARSER_ --struct-type --slot-name=keyword --global-table --null-strings --omit-struct-type --output-file=gperf-hashtable.h gperf-config.txt  */
/* Computed positions: -k'1-2' */

#if !((' ' == 32) && ('!' == 33) && ('"' == 34) && ('#' == 35) \
      && ('%' == 37) && ('&' == 38) && ('\'' == 39) && ('(' == 40) \
      && (')' == 41) && ('*' == 42) && ('+' == 43) && (',' == 44) \
      && ('-' == 45) && ('.' == 46) && ('/' == 47) && ('0' == 48) \
      && ('1' == 49) && ('2' == 50) && ('3' == 51) && ('4' == 52) \
      && ('5' == 53) && ('6' == 54) && ('7' == 55) && ('8' == 56) \
      && ('9' == 57) && (':' == 58) && (';' == 59) && ('<' == 60) \
      && ('=' == 61) && ('>' == 62) && ('?' == 63) && ('A' == 65) \
      && ('B' == 66) && ('C' == 67) && ('D' == 68) && ('E' == 69) \
      && ('F' == 70) && ('G' == 71) && ('H' == 72) && ('I' == 73) \
      && ('J' == 74) && ('K' == 75) && ('L' == 76) && ('M' == 77) \
      && ('N' == 78) && ('O' == 79) && ('P' == 80) && ('Q' == 81) \
      && ('R' == 82) && ('S' == 83) && ('T' == 84) && ('U' == 85) \
      && ('V' == 86) && ('W' == 87) && ('X' == 88) && ('Y' == 89) \
      && ('Z' == 90) && ('[' == 91) && ('\\' == 92) && (']' == 93) \
      && ('^' == 94) && ('_' == 95) && ('a' == 97) && ('b' == 98) \
      && ('c' == 99) && ('d' == 100) && ('e' == 101) && ('f' == 102) \
      && ('g' == 103) && ('h' == 104) && ('i' == 105) && ('j' == 106) \
      && ('k' == 107) && ('l' == 108) && ('m' == 109) && ('n' == 110) \
      && ('o' == 111) && ('p' == 112) && ('q' == 113) && ('r' == 114) \
      && ('s' == 115) && ('t' == 116) && ('u' == 117) && ('v' == 118) \
      && ('w' == 119) && ('x' == 120) && ('y' == 121) && ('z' == 122) \
      && ('{' == 123) && ('|' == 124) && ('}' == 125) && ('~' == 126))
/* The character set is not based on ISO-646.  */
#error "gperf generated tables don't work with this execution character set. Please report a bug to <bug-gperf@gnu.org>."
#endif


#define GPERF_PARSER_TOTAL_KEYWORDS 29
#define GPERF_PARSER_MIN_WORD_LENGTH 3
#define GPERF_PARSER_MAX_WORD_LENGTH 21
#define GPERF_PARSER_MIN_HASH_VALUE 4
#define GPERF_PARSER_MAX_HASH_VALUE 36
/* maximum key range = 33, duplicates = 0 */

#ifdef __GNUC__
__inline
#else
#ifdef __cplusplus
inline
#endif
#endif
static unsigned int
gperf_keyword_hash_function (register const char *str, register size_t len)
{
  static unsigned char asso_values[] =
    {
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 15, 10,  1,  1,  9,
       4, 37,  0, 20, 37, 37,  9, 37, 14,  0,
      37, 37,  1,  0, 37,  7, 13, 37, 18, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37, 37, 37, 37, 37,
      37, 37, 37, 37, 37, 37
    };
  return len + asso_values[(unsigned char)str[1]] + asso_values[(unsigned char)str[0]];
}

static PARSER_KEYWORD gperf_keywords[] =
  {
    {(char*)0}, {(char*)0}, {(char*)0}, {(char*)0},
#line 9 "gperf-config.txt"
    {"HOST",                   pluginsd_host,                              PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 4},
#line 42 "gperf-config.txt"
    {"RSET",                   pluginsd_replay_set,                        PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 28},
#line 17 "gperf-config.txt"
    {"CHART",                  pluginsd_chart,                             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 9},
    {(char*)0},
#line 43 "gperf-config.txt"
    {"RSSTATE",                pluginsd_replay_rrdset_collection_state,    PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 29},
#line 40 "gperf-config.txt"
    {"RDSTATE",                pluginsd_replay_rrddim_collection_state,    PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 26},
#line 12 "gperf-config.txt"
    {"HOST_LABEL",             pluginsd_host_labels,                       PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 7},
#line 10 "gperf-config.txt"
    {"HOST_DEFINE",            pluginsd_host_define,                       PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 5},
#line 26 "gperf-config.txt"
    {"SET",                    pluginsd_set,                               PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 18},
#line 33 "gperf-config.txt"
    {"SET2",                   pluginsd_set_v2,                            PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 22},
#line 41 "gperf-config.txt"
    {"REND",                   pluginsd_replay_end,                        PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 27},
#line 11 "gperf-config.txt"
    {"HOST_DEFINE_END",        pluginsd_host_define_end,                   PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 6},
#line 18 "gperf-config.txt"
    {"CLABEL",                 pluginsd_clabel,                            PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 10},
#line 39 "gperf-config.txt"
    {"RBEGIN",                 pluginsd_replay_begin,                      PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 25},
#line 6 "gperf-config.txt"
    {"FLUSH",                  pluginsd_flush,                             PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 1},
#line 22 "gperf-config.txt"
    {"FUNCTION",               pluginsd_function,                          PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 14},
#line 31 "gperf-config.txt"
    {"CLAIMED_ID",             streaming_claimed_id,                       PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 20},
#line 38 "gperf-config.txt"
    {"CHART_DEFINITION_END",   pluginsd_chart_definition_end,              PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 24},
#line 25 "gperf-config.txt"
    {"OVERWRITE",              pluginsd_overwrite,                         PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 17},
#line 19 "gperf-config.txt"
    {"CLABEL_COMMIT",          pluginsd_clabel_commit,                     PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 11},
#line 16 "gperf-config.txt"
    {"BEGIN",                  pluginsd_begin,                             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 8},
#line 32 "gperf-config.txt"
    {"BEGIN2",                 pluginsd_begin_v2,                          PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 21},
#line 21 "gperf-config.txt"
    {"END",                    pluginsd_end,                               PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 13},
#line 34 "gperf-config.txt"
    {"END2",                   pluginsd_end_v2,                            PARSER_INIT_STREAMING,                       WORKER_PARSER_FIRST_JOB + 23},
#line 7 "gperf-config.txt"
    {"DISABLE",                pluginsd_disable,                           PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 2},
#line 24 "gperf-config.txt"
    {"LABEL",                  pluginsd_label,                             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 16},
#line 20 "gperf-config.txt"
    {"DIMENSION",              pluginsd_dimension,                         PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 12},
#line 8 "gperf-config.txt"
    {"EXIT",                   pluginsd_exit,                              PARSER_INIT_PLUGINSD,                       WORKER_PARSER_FIRST_JOB + 3},
#line 23 "gperf-config.txt"
    {"FUNCTION_RESULT_BEGIN",  pluginsd_function_result_begin,             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 15},
    {(char*)0}, {(char*)0}, {(char*)0},
#line 27 "gperf-config.txt"
    {"VARIABLE",               pluginsd_variable,                          PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 19}
  };

PARSER_KEYWORD *
gperf_lookup_keyword (register const char *str, register size_t len)
{
  if (len <= GPERF_PARSER_MAX_WORD_LENGTH && len >= GPERF_PARSER_MIN_WORD_LENGTH)
    {
      register unsigned int key = gperf_keyword_hash_function (str, len);

      if (key <= GPERF_PARSER_MAX_HASH_VALUE)
        {
          register const char *s = gperf_keywords[key].keyword;

          if (s && *str == *s && !strcmp (str + 1, s + 1))
            return &gperf_keywords[key];
        }
    }
  return 0;
}
