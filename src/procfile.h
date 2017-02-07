/**
 * @file procfile.h
 * @brief Reading kernel files from /proc.
 *
 * The idea is this:
 *
 *  - every file is opened once with procfile_open().
 *
 *  - to read updated contents, we rewind it (lseek() to 0) and read again
 *    with procfile_readall().
 *
 *  - for every file, we use a buffer that is adjusted to fit its entire
 *    contents in memory, allowing us to read it with a single read() call.
 *    (this provides atomicity / consistency on the data read from the kernel)
 *
 *  - once the data are read, we update two arrays of pointers:
 *     - a words array, pointing to each word in the data read
 *     - a lines array, pointing to the first word for each line
 *
 *    This is highly optimized. Both arrays are automatically adjusted to
 *    fit all contents and are updated in a single pass on the data:
 *     - a raspberry Pi can process 5.000+ files / sec.
 *     - a J1900 celeron processor can process 23.000+ files / sec.
*/

#ifndef NETDATA_PROCFILE_H
#define NETDATA_PROCFILE_H 1

// ----------------------------------------------------------------------------
/// Array of words.
typedef struct {
    size_t len;     ///< Used entries.
    size_t size;    ///< Total entries.
    char *words[];  ///< Array of strings.
} pfwords;

// ----------------------------------------------------------------------------
/// Metadata for one line.
typedef struct {
    size_t words;     ///< Number of words the line has
    size_t first;     ///< The id of the first word of this line
                      ///< in the words array.
} ffline;

/// Collection of lines
typedef struct {
    size_t len;       ///< Used entries.
    size_t size;      ///< Total entries.
    ffline lines[];   ///< Array of lines
} pflines;

// ----------------------------------------------------------------------------
// The procfile

#define PROCFILE_FLAG_DEFAULT 0x00000000             ///< Default flag.
#define PROCFILE_FLAG_NO_ERROR_ON_FILE_IO 0x00000001 ///< Do not fail/error on file io.

/**
 * Constants used to classify a char in a procfile.
 */
typedef enum procfile_separator {
    PF_CHAR_IS_SEPARATOR,
    PF_CHAR_IS_NEWLINE,
    PF_CHAR_IS_WORD,
    PF_CHAR_IS_QUOTE,
    PF_CHAR_IS_OPEN,
    PF_CHAR_IS_CLOSE
} PF_CHAR_TYPE;

/** Procfile */
typedef struct {
    char filename[FILENAME_MAX + 1]; ///< Filename
    
    uint32_t flags;       ///< PROCFILE_FLAG_*
    int fd;               ///< File desriptor.
    size_t len;           ///< Bytes we have placed into `data`.
    size_t size;          ///< Bytes we have allocated for `data`.
    pflines *lines;       ///< Lines of the file.
    pfwords *words;       ///< Words of the file.
    char separators[256]; ///< Separator chars (procfile_separator).
    char data[];          ///< Allocated buffer to keep file contents.
} procfile;

/**
 * Close the proc file and free all related memory.
 *
 * @param ff File to close.
 */
extern void procfile_close(procfile *ff);

/**
 * Parse an open procfile.
 *
 * (re)read and parse the proc file
 *
 * @param ff File to parse.
 * @return The parsed file. `NULL` on error.
 */
extern procfile *procfile_readall(procfile *ff);

/**
 * Open a /proc or /sys file.
 *
 * @param filename to open
 * @param separators used to seperate columns
 * @param flags PROCFILE_FLAG_*
 * @return the opend procfile, `NULL` on error
 */
extern procfile *procfile_open(const char *filename, const char *separators, uint32_t flags);

/**
 * re-open a /proc or /sys file.
 *
 * If `separators == NULL`, the last separators are used
 *
 * @param ff Closed procfile.
 * @param filename to re-open
 * @param separators used to seperate columns or `NULL`
 * @param flags PROCFILE_FLAG_*
 * @return the opend procfile, `NULL` on error
 */
extern procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators, uint32_t flags);

/**
 * Example walk-through a procfile parsed file.
 *
 * @param ff File to print.
 */
extern void procfile_print(procfile *ff);

/** 
 * Add characters in quotes to the set of quotes.
 *
 * @param ff Proc file.
 * @param quotes Characters to set as quote.
 */
extern void procfile_set_quotes(procfile *ff, const char *quotes);
/**
 * Set opening and closing character used for marking multiword keywords.
 *
 * procfile supports quotes and opening / closing characters.
 * So, if you set this to `(` and `)`, the tokenizer will assume everything in parenthesis as a single keyword.
 *
 * @param ff Proc file.
 * @param open Start keyword mark.
 * @param close End keyword mark.
 */
extern void procfile_set_open_close(procfile *ff, const char *open, const char *close);

/**
 * Return the filename of procfile `ff`.
 *
 * If not set read it from proc.
 *
 * @param ff Procfile to get filename for.
 * @return the filename of `ff`
 */
extern char *procfile_filename(procfile *ff);

// ----------------------------------------------------------------------------

/**
 * @brief boolean. Adapt initial buffer allocation.
 *
 * Set this to 1, to have procfile adapt its initial buffer allocation to the max allocation used so far.
 */
extern int procfile_adaptive_initial_allocation;

/**
 * Return the number of lines present.
 *
 * @param ff Proc file.
 * @return number of lines
 */
#define procfile_lines(ff) (ff->lines->len)

/**
 * Return the number of words of the Nth line.
 *
 * @param ff Proc file.
 * @param line to count words in
 * @return number of words
 */
#define procfile_linewords(ff, line) (((line) < procfile_lines(ff)) ? (ff)->lines->lines[(line)].words : 0)

/**
 * Return the Nth word of the file, or empty string.
 *
 * @param ff Proc file.
 * @param word Number which word to return.
 * @return a word
 */
#define procfile_word(ff, word) (((word) < (ff)->words->len) ? (ff)->words->words[(word)] : "")

/**
 * Return the first word of the Nth line, or empty string.
 *
 * @param ff Proc file.
 * @param line number
 * @return First word on line `line`
 */
#define procfile_line(ff, line) (((line) < procfile_lines(ff)) ? procfile_word((ff), (ff)->lines->lines[(line)].first) : "")

/**
 * Return the Nth word of the Nth line.
 *
 * @param ff Proc file.
 * @param line number
 * @param word number
 * @param word number `word` on line `line`
 */
#define procfile_lineword(ff, line, word) (((line) < procfile_lines(ff) && (word) < procfile_linewords(ff, (line))) ? procfile_word((ff), (ff)->lines->lines[(line)].first + word) : "")

#endif /* NETDATA_PROCFILE_H */
