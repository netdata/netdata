// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PROCFILE_H
#define NETDATA_PROCFILE_H 1

#include "../libnetdata.h"

// ----------------------------------------------------------------------------
// An array of words

typedef struct {
    size_t len;     // used entries
    size_t size;    // capacity
    char *words[];  // array of pointers
} pfwords;


// ----------------------------------------------------------------------------
// An array of lines

typedef struct {
    size_t words;   // how many words this line has
    size_t first;   // the id of the first word of this line
                    // in the words array
} ffline;

typedef struct {
    size_t len;     // used entries
    size_t size;    // capacity
    ffline lines[]; // array of lines
} pflines;


// ----------------------------------------------------------------------------
// The procfile

#define PROCFILE_FLAG_DEFAULT             0x00000000
#define PROCFILE_FLAG_NO_ERROR_ON_FILE_IO 0x00000001

typedef enum procfile_separator {
    PF_CHAR_IS_SEPARATOR,
    PF_CHAR_IS_NEWLINE,
    PF_CHAR_IS_WORD,
    PF_CHAR_IS_QUOTE,
    PF_CHAR_IS_OPEN,
    PF_CHAR_IS_CLOSE
} PF_CHAR_TYPE;

typedef struct {
    char filename[FILENAME_MAX + 1]; // not populated until profile_filename() is called

    uint32_t flags;
    int fd;               // the file desriptor
    size_t len;           // the bytes we have placed into data
    size_t size;          // the bytes we have allocated for data
    pflines *lines;
    pfwords *words;
    PF_CHAR_TYPE separators[256];
    char data[];          // allocated buffer to keep file contents
} procfile;

// close the proc file and free all related memory
extern void procfile_close(procfile *ff);

// (re)read and parse the proc file
extern procfile *procfile_readall(procfile *ff);

// open a /proc or /sys file
extern procfile *procfile_open(const char *filename, const char *separators, uint32_t flags);

// re-open a file
// if separators == NULL, the last separators are used
extern procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators, uint32_t flags);

// example walk-through a procfile parsed file
extern void procfile_print(procfile *ff);

extern void procfile_set_quotes(procfile *ff, const char *quotes);
extern void procfile_set_open_close(procfile *ff, const char *open, const char *close);

extern char *procfile_filename(procfile *ff);

// ----------------------------------------------------------------------------

// set to the O_XXXX flags, to have procfile_open and procfile_reopen use them when opening proc files
extern int procfile_open_flags;

// set this to 1, to have procfile adapt its initial buffer allocation to the max allocation used so far
extern int procfile_adaptive_initial_allocation;

// return the number of lines present
#define procfile_lines(ff) ((ff)->lines->len)

// return the number of words of the Nth line
#define procfile_linewords(ff, line) (((line) < procfile_lines(ff)) ? (ff)->lines->lines[(line)].words : 0)

// return the Nth word of the file, or empty string
#define procfile_word(ff, word) (((word) < (ff)->words->len) ? (ff)->words->words[(word)] : "")

// return the first word of the Nth line, or empty string
#define procfile_line(ff, line) (((line) < procfile_lines(ff)) ? procfile_word((ff), (ff)->lines->lines[(line)].first) : "")

// return the Nth word of the current line
#define procfile_lineword(ff, line, word) (((line) < procfile_lines(ff) && (word) < procfile_linewords((ff), (line))) ? procfile_word((ff), (ff)->lines->lines[(line)].first + (word)) : "")

#endif /* NETDATA_PROCFILE_H */
