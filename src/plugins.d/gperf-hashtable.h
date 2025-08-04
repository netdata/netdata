/* ANSI-C code produced by gperf version 3.1 */
/* Command-line: gperf --multiple-iterations=1000 --output-file=gperf-hashtable.h -r gperf-config.txt  */
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

#line 1 "gperf-config.txt"


#define PLUGINSD_KEYWORD_ID_FLUSH                  97
#define PLUGINSD_KEYWORD_ID_DISABLE                98
#define PLUGINSD_KEYWORD_ID_EXIT                   99
#define PLUGINSD_KEYWORD_ID_HOST                   71
#define PLUGINSD_KEYWORD_ID_HOST_DEFINE            72
#define PLUGINSD_KEYWORD_ID_HOST_DEFINE_END        73
#define PLUGINSD_KEYWORD_ID_HOST_LABEL             74

#define PLUGINSD_KEYWORD_ID_BEGIN                  12
#define PLUGINSD_KEYWORD_ID_CHART                  32
#define PLUGINSD_KEYWORD_ID_CLABEL                 34
#define PLUGINSD_KEYWORD_ID_CLABEL_COMMIT          35
#define PLUGINSD_KEYWORD_ID_DIMENSION              31
#define PLUGINSD_KEYWORD_ID_END                    13
#define PLUGINSD_KEYWORD_ID_FUNCTION               41
#define PLUGINSD_KEYWORD_ID_FUNCTION_RESULT_BEGIN  42
#define PLUGINSD_KEYWORD_ID_FUNCTION_PROGRESS      43
#define PLUGINSD_KEYWORD_ID_LABEL                  51
#define PLUGINSD_KEYWORD_ID_OVERWRITE              52
#define PLUGINSD_KEYWORD_ID_SET                    11
#define PLUGINSD_KEYWORD_ID_VARIABLE               53
#define PLUGINSD_KEYWORD_ID_CONFIG                 100
#define PLUGINSD_KEYWORD_ID_TRUST_DURATIONS        101

#define PLUGINSD_KEYWORD_ID_CLAIMED_ID             61
#define PLUGINSD_KEYWORD_ID_BEGIN2                 2
#define PLUGINSD_KEYWORD_ID_SET2                   1
#define PLUGINSD_KEYWORD_ID_END2                   3

#define PLUGINSD_KEYWORD_ID_CHART_DEFINITION_END   33
#define PLUGINSD_KEYWORD_ID_RBEGIN                 22
#define PLUGINSD_KEYWORD_ID_RDSTATE                23
#define PLUGINSD_KEYWORD_ID_REND                   25
#define PLUGINSD_KEYWORD_ID_RSET                   21
#define PLUGINSD_KEYWORD_ID_RSSTATE                24

#define PLUGINSD_KEYWORD_ID_JSON                   80

#define PLUGINSD_KEYWORD_ID_DYNCFG_ENABLE          901
#define PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_MODULE 902
#define PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_JOB    903
#define PLUGINSD_KEYWORD_ID_DYNCFG_RESET           904
#define PLUGINSD_KEYWORD_ID_REPORT_JOB_STATUS      905
#define PLUGINSD_KEYWORD_ID_DELETE_JOB             906


#define GPERF_PARSER_TOTAL_KEYWORDS 39
#define GPERF_PARSER_MIN_WORD_LENGTH 3
#define GPERF_PARSER_MAX_WORD_LENGTH 22
#define GPERF_PARSER_MIN_HASH_VALUE 4
#define GPERF_PARSER_MAX_HASH_VALUE 53
/* maximum key range = 50, duplicates = 0 */

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
  static const unsigned char asso_values[] =
    {
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 31, 28,  2,  4,  0,
       5, 54,  0, 25, 22, 54, 17, 54, 27,  0,
      54, 54,  1, 16, 24, 15,  0, 54,  2,  0,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54, 54, 54, 54, 54,
      54, 54, 54, 54, 54, 54
    };
  return len + asso_values[(unsigned char)str[1]] + asso_values[(unsigned char)str[0]];
}

static const PARSER_KEYWORD gperf_keywords[] =
  {
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
#line 70 "gperf-config.txt"
    {"HOST",            PLUGINSD_KEYWORD_ID_HOST,            PARSER_INIT_PLUGINSD|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 4},
#line 105 "gperf-config.txt"
    {"REND",                 PLUGINSD_KEYWORD_ID_REND,                 PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 30},
#line 69 "gperf-config.txt"
    {"EXIT",            PLUGINSD_KEYWORD_ID_EXIT,            PARSER_INIT_PLUGINSD,                     WORKER_PARSER_FIRST_JOB + 3},
#line 78 "gperf-config.txt"
    {"CHART",                 PLUGINSD_KEYWORD_ID_CHART,                 PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA|PARSER_REP_REPLICATION, WORKER_PARSER_FIRST_JOB + 9},
#line 90 "gperf-config.txt"
    {"CONFIG",                PLUGINSD_KEYWORD_ID_CONFIG,                PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                       WORKER_PARSER_FIRST_JOB + 21},
#line 87 "gperf-config.txt"
    {"OVERWRITE",             PLUGINSD_KEYWORD_ID_OVERWRITE,             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 18},
#line 73 "gperf-config.txt"
    {"HOST_LABEL",      PLUGINSD_KEYWORD_ID_HOST_LABEL,      PARSER_INIT_PLUGINSD|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 7},
#line 71 "gperf-config.txt"
    {"HOST_DEFINE",     PLUGINSD_KEYWORD_ID_HOST_DEFINE,     PARSER_INIT_PLUGINSD|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 5},
#line 106 "gperf-config.txt"
    {"RDSTATE",              PLUGINSD_KEYWORD_ID_RDSTATE,              PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 31},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
#line 120 "gperf-config.txt"
    {"DELETE_JOB",             PLUGINSD_KEYWORD_ID_DELETE_JOB,             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 39},
#line 72 "gperf-config.txt"
    {"HOST_DEFINE_END", PLUGINSD_KEYWORD_ID_HOST_DEFINE_END, PARSER_INIT_PLUGINSD|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 6},
#line 118 "gperf-config.txt"
    {"DYNCFG_RESET",           PLUGINSD_KEYWORD_ID_DYNCFG_RESET,           PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 37},
#line 115 "gperf-config.txt"
    {"DYNCFG_ENABLE",          PLUGINSD_KEYWORD_ID_DYNCFG_ENABLE,          PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 34},
#line 119 "gperf-config.txt"
    {"REPORT_JOB_STATUS",      PLUGINSD_KEYWORD_ID_REPORT_JOB_STATUS,      PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 38},
#line 88 "gperf-config.txt"
    {"SET",                   PLUGINSD_KEYWORD_ID_SET,                   PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 19},
#line 97 "gperf-config.txt"
    {"SET2",       PLUGINSD_KEYWORD_ID_SET2,       PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 25},
#line 104 "gperf-config.txt"
    {"RSET",                 PLUGINSD_KEYWORD_ID_RSET,                 PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 29},
#line 102 "gperf-config.txt"
    {"CHART_DEFINITION_END", PLUGINSD_KEYWORD_ID_CHART_DEFINITION_END, PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 27},
#line 117 "gperf-config.txt"
    {"DYNCFG_REGISTER_JOB",    PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_JOB,    PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 36},
#line 107 "gperf-config.txt"
    {"RSSTATE",              PLUGINSD_KEYWORD_ID_RSSTATE,              PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 32},
#line 79 "gperf-config.txt"
    {"CLABEL",                PLUGINSD_KEYWORD_ID_CLABEL,                PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 10},
#line 116 "gperf-config.txt"
    {"DYNCFG_REGISTER_MODULE", PLUGINSD_KEYWORD_ID_DYNCFG_REGISTER_MODULE, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING, WORKER_PARSER_FIRST_JOB + 35},
#line 67 "gperf-config.txt"
    {"FLUSH",           PLUGINSD_KEYWORD_ID_FLUSH,           PARSER_INIT_PLUGINSD,                     WORKER_PARSER_FIRST_JOB + 1},
#line 83 "gperf-config.txt"
    {"FUNCTION",              PLUGINSD_KEYWORD_ID_FUNCTION,              PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 14},
#line 95 "gperf-config.txt"
    {"CLAIMED_ID", PLUGINSD_KEYWORD_ID_CLAIMED_ID, PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 23},
#line 82 "gperf-config.txt"
    {"END",                   PLUGINSD_KEYWORD_ID_END,                   PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 13},
#line 98 "gperf-config.txt"
    {"END2",       PLUGINSD_KEYWORD_ID_END2,       PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 26},
#line 80 "gperf-config.txt"
    {"CLABEL_COMMIT",         PLUGINSD_KEYWORD_ID_CLABEL_COMMIT,         PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 11},
#line 77 "gperf-config.txt"
    {"BEGIN",                 PLUGINSD_KEYWORD_ID_BEGIN,                 PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 8},
#line 96 "gperf-config.txt"
    {"BEGIN2",     PLUGINSD_KEYWORD_ID_BEGIN2,     PARSER_INIT_STREAMING|PARSER_REP_DATA,     WORKER_PARSER_FIRST_JOB + 24},
#line 103 "gperf-config.txt"
    {"RBEGIN",               PLUGINSD_KEYWORD_ID_RBEGIN,               PARSER_INIT_STREAMING|PARSER_REP_REPLICATION|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 28},
#line 68 "gperf-config.txt"
    {"DISABLE",         PLUGINSD_KEYWORD_ID_DISABLE,         PARSER_INIT_PLUGINSD,                     WORKER_PARSER_FIRST_JOB + 2},
#line 85 "gperf-config.txt"
    {"FUNCTION_PROGRESS",     PLUGINSD_KEYWORD_ID_FUNCTION_PROGRESS,     PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                     WORKER_PARSER_FIRST_JOB + 16},
#line 81 "gperf-config.txt"
    {"DIMENSION",             PLUGINSD_KEYWORD_ID_DIMENSION,             PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 12},
#line 89 "gperf-config.txt"
    {"VARIABLE",              PLUGINSD_KEYWORD_ID_VARIABLE,              PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 20},
#line 91 "gperf-config.txt"
    {"TRUST_DURATIONS",       PLUGINSD_KEYWORD_ID_TRUST_DURATIONS,       PARSER_INIT_PLUGINSD|PARSER_REP_METADATA,                       WORKER_PARSER_FIRST_JOB + 22},
#line 84 "gperf-config.txt"
    {"FUNCTION_RESULT_BEGIN", PLUGINSD_KEYWORD_ID_FUNCTION_RESULT_BEGIN, PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING,                     WORKER_PARSER_FIRST_JOB + 15},
#line 111 "gperf-config.txt"
    {"JSON",                 PLUGINSD_KEYWORD_ID_JSON,                 PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 33},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
    {(char*)0,0,PARSER_INIT_PLUGINSD,0},
#line 86 "gperf-config.txt"
    {"LABEL",                 PLUGINSD_KEYWORD_ID_LABEL,                 PARSER_INIT_PLUGINSD|PARSER_INIT_STREAMING|PARSER_REP_METADATA, WORKER_PARSER_FIRST_JOB + 17}
  };

const PARSER_KEYWORD *
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
