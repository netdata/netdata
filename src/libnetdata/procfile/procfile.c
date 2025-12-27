// SPDX-License-Identifier: GPL-3.0-or-later

#include "../libnetdata.h"

#define PF_PREFIX "PROCFILE"

#define PFWORDS_INCREASE_STEP 2000
#define PFLINES_INCREASE_STEP 200
#define PROCFILE_INCREMENT_BUFFER 4096

int procfile_open_flags = O_RDONLY | O_CLOEXEC;

// if adaptive allocation is set, these store the
// max values we have seen so far
static bool procfile_adaptive_initial_allocation = false;
static size_t procfile_max_lines = PFLINES_INCREASE_STEP;
static size_t procfile_max_words = PFWORDS_INCREASE_STEP;
static size_t procfile_max_allocation = PROCFILE_INCREMENT_BUFFER;

void procfile_set_adaptive_allocation(bool enable, size_t bytes, size_t lines, size_t words) {
    procfile_adaptive_initial_allocation = enable;

    if(bytes > procfile_max_allocation)
        procfile_max_allocation = bytes;
    if(lines > procfile_max_lines)
        procfile_max_lines = lines;
    if(words > procfile_max_words)
        procfile_max_words = words;
}

// ----------------------------------------------------------------------------

char *procfile_filename(procfile *ff) {
    if(ff->filename)
        return ff->filename;

    char filename[FILENAME_MAX + 1];
    char buffer[FILENAME_MAX + 1];
    snprintfz(buffer, FILENAME_MAX, "/proc/self/fd/%d", ff->fd);

    ssize_t l = readlink(buffer, filename, FILENAME_MAX);
    if(unlikely(l == -1))
        snprintfz(filename, FILENAME_MAX, "unknown filename for fd %d", ff->fd);
    else
        filename[l] = '\0';


    ff->filename = strdupz(filename);

    // on non-linux systems, something like this will be needed
    // fcntl(ff->fd, F_GETPATH, ff->filename)

    return ff->filename;
}

// ----------------------------------------------------------------------------
// An array of words

static inline void procfile_words_add(procfile *ff, char *str) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   adding word No %d: '%s'", fw->len, str);

    pfwords *fw = ff->words;
    if(unlikely(fw->len == fw->size)) {
        // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   expanding words");
        size_t minimum = PFWORDS_INCREASE_STEP;
        size_t optimal = fw->size / 2;
        size_t wanted = (optimal > minimum)?optimal:minimum;

        ff->words = fw = reallocz(fw, sizeof(pfwords) + (fw->size + wanted) * sizeof(char *));
        fw->size += wanted;
        ff->stats.memory += wanted * sizeof(char *);
        ff->stats.resizes++;
    }

    fw->words[fw->len++] = str;
}

NEVERNULL
static inline pfwords *procfile_words_create(void) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   initializing words");

    size_t size = (procfile_adaptive_initial_allocation) ? procfile_max_words : PFWORDS_INCREASE_STEP;

    pfwords *new = mallocz(sizeof(pfwords) + size * sizeof(char *));
    new->len = 0;
    new->size = size;
    return new;
}

static inline void procfile_words_reset(pfwords *fw) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   resetting words");
    fw->len = 0;
}

static inline void procfile_words_free(pfwords *fw) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   freeing words");

    freez(fw);
}


// ----------------------------------------------------------------------------
// An array of lines

NEVERNULL
static inline uint32_t *procfile_lines_add(procfile *ff) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   adding line %d at word %d", fl->len, first_word);

    pflines *fl = ff->lines;
    if(unlikely(fl->len == fl->size)) {
        // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   expanding lines");
        size_t minimum = PFLINES_INCREASE_STEP;
        size_t optimal = fl->size / 2;
        size_t wanted = (optimal > minimum)?optimal:minimum;

        ff->lines = fl = reallocz(fl, sizeof(pflines) + (fl->size + wanted) * sizeof(ffline));
        fl->size += wanted;
        ff->stats.memory += wanted * sizeof(ffline);
        ff->stats.resizes++;
    }

    ffline *ffl = &fl->lines[fl->len++];
    ffl->words = 0;
    ffl->first = ff->words->len;

    return &ffl->words;
}

NEVERNULL
static inline pflines *procfile_lines_create(void) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   initializing lines");

    size_t size = (unlikely(procfile_adaptive_initial_allocation)) ? procfile_max_words : PFLINES_INCREASE_STEP;

    pflines *new = mallocz(sizeof(pflines) + size * sizeof(ffline));
    new->len = 0;
    new->size = size;
    return new;
}

static inline void procfile_lines_reset(pflines *fl) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   resetting lines");

    fl->len = 0;
}

static inline void procfile_lines_free(pflines *fl) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   freeing lines");

    freez(fl);
}


// ----------------------------------------------------------------------------
// The procfile

void procfile_close(procfile *ff) {
    if(unlikely(!ff)) return;

    netdata_log_debug(D_PROCFILE, PF_PREFIX ": Closing file '%s'", procfile_filename(ff));

    freez(ff->filename);
    procfile_lines_free(ff->lines);
    procfile_words_free(ff->words);

    if(likely(ff->fd != -1)) close(ff->fd);
    freez(ff);
}

NOINLINE
static void procfile_parser(procfile *ff) {
    // netdata_log_debug(D_PROCFILE, PF_PREFIX ": Parsing file '%s'", ff->filename);

    char  *s = ff->data                 // our current position
        , *e = &ff->data[ff->len]       // the terminating null
        , *t = ff->data;                // the first character of a word (or quoted / parenthesized string)

                                        // the look up array to find our type of character
    PF_CHAR_TYPE *separators = ff->separators;

    char quote = 0;                     // the quote character - only when in quoted string
    size_t opened = 0;                  // counts the number of open parenthesis

    uint32_t *line_words = procfile_lines_add(ff);

    while(s < e) {
        PF_CHAR_TYPE ct = separators[(unsigned char)(*s)];

        // this is faster than a switch()
        // read more here: http://lazarenko.me/switch/
        if(likely(ct == PF_CHAR_IS_WORD)) {
            s++;
        }
        else if(likely(ct == PF_CHAR_IS_SEPARATOR)) {
            if(!quote && !opened) {
                if (s != t) {
                    // separator, but we have word before it
                    *s = '\0';
                    procfile_words_add(ff, t);
                    (*line_words)++;
                    t = ++s;
                }
                else {
                    // separator at the beginning
                    // skip it
                    t = ++s;
                }
            }
            else {
                // we are inside a quote or parenthesized string
                s++;
            }
        }
        else if(likely(ct == PF_CHAR_IS_NEWLINE)) {
            // end of line

            *s = '\0';
            procfile_words_add(ff, t);
            (*line_words)++;
            t = ++s;

            // netdata_log_debug(D_PROCFILE, PF_PREFIX ":   ended line %d with %d words", l, ff->lines->lines[l].words);

            line_words = procfile_lines_add(ff);
        }
        else if(likely(ct == PF_CHAR_IS_QUOTE)) {
            if(unlikely(!quote && s == t)) {
                // quote opened at the beginning
                quote = *s;
                t = ++s;
            }
            else if(unlikely(quote && quote == *s)) {
                // quote closed
                quote = 0;

                *s = '\0';
                procfile_words_add(ff, t);
                (*line_words)++;
                t = ++s;
            }
            else
                s++;
        }
        else if(likely(ct == PF_CHAR_IS_OPEN)) {
            if(s == t) {
                if(!opened)
                    t = ++s;
                else
                    ++s;

                opened++;
            }
            else if(opened) {
                opened++;
                s++;
            }
            else
                s++;
        }
        else if(likely(ct == PF_CHAR_IS_CLOSE)) {
            if(opened) {
                opened--;

                if(!opened) {
                    *s = '\0';
                    procfile_words_add(ff, t);
                    (*line_words)++;
                    t = ++s;
                }
                else
                    s++;
            }
            else
                s++;
        }
        else
            fatal("Internal Error: procfile_readall() does not handle all the cases.");
    }

    if(likely(s > t && t < e)) {
        // the last word
        if(unlikely(ff->len >= ff->size)) {
            // we are going to loose the last byte
            s = &ff->data[ff->size - 1];
        }

        *s = '\0';
        procfile_words_add(ff, t);
        (*line_words)++;
        // t = ++s;
    }
}

procfile *procfile_readall(procfile *ff) {
    if(!ff) return NULL;

    // netdata_log_debug(D_PROCFILE, PF_PREFIX ": Reading file '%s'.", ff->filename);

    ff->len = 0;    // zero the used size
    ssize_t r = 1;  // read at least once
    while(r > 0) {
        ssize_t s = ff->len;
        ssize_t x = ff->size - s;

        if(unlikely(!x)) {
            size_t minimum = PROCFILE_INCREMENT_BUFFER;
            size_t optimal = ff->size / 2;
            size_t wanted = (optimal > minimum)?optimal:minimum;

            netdata_log_debug(D_PROCFILE, PF_PREFIX ": Expanding data buffer for file '%s' by %zu bytes.", procfile_filename(ff), wanted);
            ff = reallocz(ff, sizeof(procfile) + ff->size + wanted);
            ff->size += wanted;
            ff->stats.memory += wanted;
            ff->stats.resizes++;
        }

        // netdata_log_info("Reading file '%s', from position %zd with length %zd", procfile_filename(ff), s, (ssize_t)(ff->size - s));
        ff->stats.reads++;
        r = read(ff->fd, &ff->data[s], ff->size - s);
        if(unlikely(r == -1)) {
            if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) collector_error(PF_PREFIX ": Cannot read from file '%s' on fd %d", procfile_filename(ff), ff->fd);
            else if(unlikely(ff->flags & PROCFILE_FLAG_ERROR_ON_ERROR_LOG))
                netdata_log_error(PF_PREFIX ": Cannot read from file '%s' on fd %d", procfile_filename(ff), ff->fd);
            procfile_close(ff);
            return NULL;
        }

        if((ssize_t)ff->stats.max_read_size < r)
            ff->stats.max_read_size = r;

        ff->len += r;
    }

    // netdata_log_debug(D_PROCFILE, "Rewinding file '%s'", ff->filename);
    
    // Skip lseek if we already know this file is non-seekable
    if(unlikely(ff->flags & PROCFILE_FLAG_NONSEEKABLE)) {
        // Reopen to reset position
        char *fn = strdupz(procfile_filename(ff));
        ff = procfile_reopen(ff, fn, NULL, ff->flags);
        freez(fn);
        if(unlikely(!ff))
            return NULL;
    }
    else if(unlikely(lseek(ff->fd, 0, SEEK_SET) == -1)) {
        // Some procfs files (Ubuntu HWE 24.04 / kernel 6.14) may be non-seekable.
        // In that case, "rewind" by reopening.
        if(errno == ESPIPE || errno == EINVAL) {
            // Mark this file as non-seekable to avoid future lseek attempts
            ff->flags |= PROCFILE_FLAG_NONSEEKABLE;
            
            // Must duplicate the filename before reopen frees it
            char *fn = strdupz(procfile_filename(ff));
            // Reopen resets file position to 0.
            ff = procfile_reopen(ff, fn, NULL, ff->flags);
            freez(fn);
            if(unlikely(!ff))
                return NULL;
        }
        else {
            if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO)))
                collector_error(PF_PREFIX ": Cannot rewind on file '%s'.", procfile_filename(ff));
            else if(unlikely(ff->flags & PROCFILE_FLAG_ERROR_ON_ERROR_LOG))
                netdata_log_error(PF_PREFIX ": Cannot rewind on file '%s'.", procfile_filename(ff));
            procfile_close(ff);
            return NULL;
        }
    }

    procfile_lines_reset(ff->lines);
    procfile_words_reset(ff->words);
    procfile_parser(ff);

    if(unlikely(procfile_adaptive_initial_allocation)) {
        if(unlikely(ff->len > procfile_max_allocation)) procfile_max_allocation = ff->len;
        if(unlikely(ff->lines->len > procfile_max_lines)) procfile_max_lines = ff->lines->len;
        if(unlikely(ff->words->len > procfile_max_words)) procfile_max_words = ff->words->len;
    }

    if(ff->stats.max_source_bytes < ff->len)
        ff->stats.max_source_bytes = ff->len;

    if(ff->stats.max_lines < ff->lines->len)
        ff->stats.max_lines = ff->lines->len;

    if(ff->stats.max_words < ff->words->len)
        ff->stats.max_words = ff->words->len;

    ff->stats.total_read_bytes += ff->len;

    // netdata_log_debug(D_PROCFILE, "File '%s' updated.", ff->filename);
    return ff;
}

static PF_CHAR_TYPE procfile_default_separators[256];
__attribute__((constructor)) void procfile_initialize_default_separators(void) {
    int i = 256;
    while(i--) {
        if(unlikely(i == '\n' || i == '\r'))
            procfile_default_separators[i] = PF_CHAR_IS_NEWLINE;

        else if(unlikely(isspace(i) || (!isprint(i) && !IS_UTF8_BYTE(i))))
            procfile_default_separators[i] = PF_CHAR_IS_SEPARATOR;

        else
            procfile_default_separators[i] = PF_CHAR_IS_WORD;
    }
}

NOINLINE
static void procfile_set_separators(procfile *ff, const char *separators) {
    // set the separators
    if(unlikely(!separators))
        separators = " \t=|";

    // copy the default
    memcpy(ff->separators, procfile_default_separators, 256 * sizeof(PF_CHAR_TYPE));

    PF_CHAR_TYPE *ffs = ff->separators;
    const char *s = separators;
    while(*s)
        ffs[(int)*s++] = PF_CHAR_IS_SEPARATOR;
}

void procfile_set_quotes(procfile *ff, const char *quotes) {
    PF_CHAR_TYPE *ffs = ff->separators;

    // remove all quotes
    int i = 256;
    while(i--)
        if(unlikely(ffs[i] == PF_CHAR_IS_QUOTE))
            ffs[i] = PF_CHAR_IS_WORD;

    // if nothing given, return
    if(unlikely(!quotes || !*quotes))
        return;

    // set the quotes
    const char *s = quotes;
    while(*s)
        ffs[(int)*s++] = PF_CHAR_IS_QUOTE;
}

void procfile_set_open_close(procfile *ff, const char *open, const char *close) {
    PF_CHAR_TYPE *ffs = ff->separators;

    // remove all open/close
    int i = 256;
    while(i--)
        if(unlikely(ffs[i] == PF_CHAR_IS_OPEN || ffs[i] == PF_CHAR_IS_CLOSE))
            ffs[i] = PF_CHAR_IS_WORD;

    // if nothing given, return
    if(unlikely(!open || !*open || !close || !*close))
        return;

    // set the openings
    const char *s = open;
    while(*s)
        ffs[(int)*s++] = PF_CHAR_IS_OPEN;

    // set the closings
    s = close;
    while(*s)
        ffs[(int)*s++] = PF_CHAR_IS_CLOSE;
}

procfile *procfile_open(const char *filename, const char *separators, uint32_t flags) {
    netdata_log_debug(D_PROCFILE, PF_PREFIX ": Opening file '%s'", filename);

    int fd = open(filename, procfile_open_flags, 0666);
    if(unlikely(fd == -1)) {
        if (unlikely(flags & PROCFILE_FLAG_ERROR_ON_ERROR_LOG))
            netdata_log_error(PF_PREFIX ": Cannot open file '%s'", filename);
        else if (unlikely(!(flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) {
            if (errno == ENOENT)
                collector_info(PF_PREFIX ": Cannot open file '%s'", filename);
            else
                collector_error(PF_PREFIX ": Cannot open file '%s'", filename);
        }
        return NULL;
    }

    // netdata_log_info("PROCFILE: opened '%s' on fd %d", filename, fd);

    size_t size = (unlikely(procfile_adaptive_initial_allocation)) ? procfile_max_allocation : PROCFILE_INCREMENT_BUFFER;
    procfile *ff = mallocz(sizeof(procfile) + size);

    //strncpyz(ff->filename, filename, FILENAME_MAX);
    ff->filename = NULL;
    ff->fd = fd;
    ff->size = size;
    ff->len = 0;
    ff->flags = flags;
    ff->stats.opens = 1;
    ff->stats.reads = ff->stats.resizes = 0;
    ff->stats.max_lines = ff->stats.max_words = ff->stats.max_source_bytes = 0;
    ff->stats.total_read_bytes = ff->stats.max_read_size = 0;

    ff->lines = procfile_lines_create();
    ff->words = procfile_words_create();

    ff->stats.memory = sizeof(procfile) + size +
                       (sizeof(pflines) + ff->lines->size * sizeof(ffline)) +
                       (sizeof(pfwords) + ff->words->size * sizeof(char *));

    procfile_set_separators(ff, separators);

    netdata_log_debug(D_PROCFILE, "File '%s' opened.", filename);
    return ff;
}

procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators, uint32_t flags) {
    if(unlikely(!ff)) return procfile_open(filename, separators, flags);

    if(likely(ff->fd != -1)) {
        // netdata_log_info("PROCFILE: closing fd %d", ff->fd);
        close(ff->fd);
    }

    ff->fd = open(filename, procfile_open_flags, 0666);
    if(unlikely(ff->fd == -1)) {
        procfile_close(ff);
        return NULL;
    }
    ff->stats.opens++;

    // netdata_log_info("PROCFILE: opened '%s' on fd %d", filename, ff->fd);

    //strncpyz(ff->filename, filename, FILENAME_MAX);
    freez(ff->filename);
    ff->filename = NULL;
    ff->flags = flags;

    // do not do the separators again if NULL is given
    if(likely(separators)) procfile_set_separators(ff, separators);

    return ff;
}

// ----------------------------------------------------------------------------
// example parsing of procfile data

void procfile_print(procfile *ff) {
    size_t lines = procfile_lines(ff), l;
    char *s;
    (void)s;

    netdata_log_debug(D_PROCFILE, "File '%s' with %zu lines and %zu words", procfile_filename(ff), ff->lines->len, ff->words->len);

    for(l = 0; likely(l < lines) ;l++) {
        size_t words = procfile_linewords(ff, l);

        netdata_log_debug(D_PROCFILE, " line %zu starts at word %zu and has %zu words", l, (size_t)ff->lines->lines[l].first, (size_t)ff->lines->lines[l].words);

        size_t w;
        for(w = 0; likely(w < words) ;w++) {
            s = procfile_lineword(ff, l, w);
            netdata_log_debug(D_PROCFILE, "     [%zu.%zu] '%s'", l, w, s);
        }
    }
}
