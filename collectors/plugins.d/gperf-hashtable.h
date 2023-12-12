/* ANSI-C code produced by gperf version 3.1 */
/* Command-line: gperf --multiple-iterations=1000 --output-file=gperf-hashtable.h gperf-config.txt  */
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


#define GPERF_PARSER_TOTAL_KEYWORDS 36
#define GPERF_PARSER_MIN_WORD_LENGTH 3
#define GPERF_PARSER_MAX_WORD_LENGTH 22
#define GPERF_PARSER_MIN_HASH_VALUE 3
#define GPERF_PARSER_MAX_HASH_VALUE 48
/* maximum key range = 46, duplicates = 0 */

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
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 23, 29,  0,  0,  0,
       0, 49,  9,  0, 49, 49, 20, 49,  0,  8,
      49, 49,  1, 12, 49, 23,  6, 49,  2,  0,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49, 49, 49, 49, 49,
      49, 49, 49, 49, 49, 49
    };
  return len + asso_values[(unsigned char)str[1]] + asso_values[(unsigned char)str[0]];
}

static PARSER_KEYWORD gperf_keywords[] =
  {
    {(char*)0}, {(char*)0}, {(char*)0},
#line 30 "gperf-config.txt"
    {"END",                     13, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 13},
#line 51 "gperf-config.txt"
    {"END2",                     3, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 30},
#line 58 "gperf-config.txt"
    {"REND",                    25, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 34},
#line 17 "gperf-config.txt"
    {"EXIT",                    99, PARSER_INIT_PLUGINSD,                                              WORKER_PARSER_FIRST_JOB + 3},
#line 16 "gperf-config.txt"
    {"DISABLE",                 98, PARSER_INIT_PLUGINSD,                                              WORKER_PARSER_FIRST_JOB + 2},
#line 57 "gperf-config.txt"
    {"RDSTATE",                 23, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 33},
#line 29 "gperf-config.txt"
    {"DIMENSION",               31, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 12},
#line 44 "gperf-config.txt"
    {"DELETE_JOB",             111, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 26},
    {(char*)0},
#line 42 "gperf-config.txt"
    {"DYNCFG_RESET",           104, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 24},
#line 39 "gperf-config.txt"
    {"DYNCFG_ENABLE",          101, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 21},
#line 26 "gperf-config.txt"
    {"CHART",                   32, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 9},
#line 37 "gperf-config.txt"
    {"SET",                     11, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 19},
#line 50 "gperf-config.txt"
    {"SET2",                     1, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 29},
#line 59 "gperf-config.txt"
    {"RSET",                    21, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 35},
#line 43 "gperf-config.txt"
    {"REPORT_JOB_STATUS",      110, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 25},
#line 41 "gperf-config.txt"
    {"DYNCFG_REGISTER_JOB",    103, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 23},
#line 60 "gperf-config.txt"
    {"RSSTATE",                 24, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 36},
#line 18 "gperf-config.txt"
    {"HOST",                    71, PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                          WORKER_PARSER_FIRST_JOB + 4},
#line 40 "gperf-config.txt"
    {"DYNCFG_REGISTER_MODULE", 102, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 22},
#line 36 "gperf-config.txt"
    {"OVERWRITE",               52, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 18},
    {(char*)0},
#line 15 "gperf-config.txt"
    {"FLUSH",                   97, PARSER_INIT_PLUGINSD,                                              WORKER_PARSER_FIRST_JOB + 1},
#line 27 "gperf-config.txt"
    {"CLABEL",                  34, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 10},
#line 21 "gperf-config.txt"
    {"HOST_LABEL",              74, PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                          WORKER_PARSER_FIRST_JOB + 7},
#line 19 "gperf-config.txt"
    {"HOST_DEFINE",             72, PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                          WORKER_PARSER_FIRST_JOB + 5},
#line 55 "gperf-config.txt"
    {"CHART_DEFINITION_END",    33, PARSER_INIT_STREAMING|PARSER_REP_METADATA,                         WORKER_PARSER_FIRST_JOB + 31},
#line 48 "gperf-config.txt"
    {"CLAIMED_ID",              61, PARSER_INIT_STREAMING|PARSER_REP_METADATA,                         WORKER_PARSER_FIRST_JOB + 27},
#line 31 "gperf-config.txt"
    {"FUNCTION",                41, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 14},
#line 20 "gperf-config.txt"
    {"HOST_DEFINE_END",         73, PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                          WORKER_PARSER_FIRST_JOB + 6},
#line 28 "gperf-config.txt"
    {"CLABEL_COMMIT",           35, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 11},
#line 25 "gperf-config.txt"
    {"BEGIN",                   12, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 8},
#line 49 "gperf-config.txt"
    {"BEGIN2",                   2, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 28},
#line 56 "gperf-config.txt"
    {"RBEGIN",                  22, PARSER_INIT_STREAMING,                                             WORKER_PARSER_FIRST_JOB + 32},
#line 38 "gperf-config.txt"
    {"VARIABLE",                53, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 20},
    {(char*)0}, {(char*)0},
#line 33 "gperf-config.txt"
    {"FUNCTION_PROGRESS",       43, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 16},
    {(char*)0}, {(char*)0}, {(char*)0},
#line 32 "gperf-config.txt"
    {"FUNCTION_RESULT_BEGIN",   42, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                        WORKER_PARSER_FIRST_JOB + 15},
    {(char*)0}, {(char*)0}, {(char*)0},
#line 35 "gperf-config.txt"
    {"LABEL",                   51, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA,    WORKER_PARSER_FIRST_JOB + 17}
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
