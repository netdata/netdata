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
    uint32_t words; // how many words this line has
    uint32_t first; // the id of the first word of this line in the words array
} ffline;

typedef struct {
    size_t len;     // used entries
    size_t size;    // capacity
    ffline lines[]; // array of lines
} pflines;


// ----------------------------------------------------------------------------
// The procfile

#define PROCFILE_FLAG_DEFAULT             0x00000000 // To store inside `collector.log`
#define PROCFILE_FLAG_NO_ERROR_ON_FILE_IO 0x00000001 // Do not log anything
#define PROCFILE_FLAG_ERROR_ON_ERROR_LOG  0x00000002 // Store inside `error.log`
#define PROCFILE_FLAG_NONSEEKABLE         0x00000004 // File doesn't support lseek(), reopen instead

typedef enum __attribute__ ((__packed__)) procfile_separator {
    PF_CHAR_IS_SEPARATOR,
    PF_CHAR_IS_NEWLINE,
    PF_CHAR_IS_WORD,
    PF_CHAR_IS_QUOTE,
    PF_CHAR_IS_OPEN,
    PF_CHAR_IS_CLOSE
} PF_CHAR_TYPE;

struct procfile_stats {
    size_t opens;
    size_t reads;
    size_t resizes;
    size_t memory;
    size_t total_read_bytes;
    size_t max_source_bytes;
    size_t max_lines;
    size_t max_words;
    size_t max_read_size;
};


typedef struct procfile {
    // this structure is malloc'd (you need to initialize it at procfile_open()

    char *filename;                 // not populated until procfile_filename() is called
    uint32_t flags;
    int fd;                         // the file descriptor
    size_t len;                     // the bytes we have placed into data
    size_t size;                    // the bytes we have allocated for data
    pflines *lines;
    pfwords *words;
    PF_CHAR_TYPE separators[256];
    struct procfile_stats stats;
    char data[];                    // allocated buffer to keep file contents
} procfile;

// close the proc file and free all related memory
void procfile_close(procfile *ff);

// (re)read and parse the proc file
procfile *procfile_readall(procfile *ff);

// open a /proc or /sys file
procfile *procfile_open(const char *filename, const char *separators, uint32_t flags);

// re-open a file
// if separators == NULL, the last separators are used
procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators, uint32_t flags);

// example walk-through a procfile parsed file
void procfile_print(procfile *ff);

void procfile_set_quotes(procfile *ff, const char *quotes);
void procfile_set_open_close(procfile *ff, const char *open, const char *close);

char *procfile_filename(procfile *ff);

// ----------------------------------------------------------------------------

// set to the O_XXXX flags, to have procfile_open and procfile_reopen use them when opening proc files
extern int procfile_open_flags;

// call this with true and the expected initial sizes to allow procfile learn the sizes needed
void procfile_set_adaptive_allocation(bool enable, size_t bytes, size_t lines, size_t words);

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

// Open file without logging file IO error if any
#define procfile_open_no_log(filename, separators, flags) procfile_open(filename, separators, flags | PROCFILE_FLAG_NO_ERROR_ON_FILE_IO)

#endif /* NETDATA_PROCFILE_H */
