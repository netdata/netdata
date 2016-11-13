#include "common.h"

#define PF_PREFIX "PROCFILE"

#define PFWORDS_INCREASE_STEP 200
#define PFLINES_INCREASE_STEP 10
#define PROCFILE_INCREMENT_BUFFER 512

int procfile_adaptive_initial_allocation = 0;

// if adaptive allocation is set, these store the
// max values we have seen so far
uint32_t procfile_max_lines = PFLINES_INCREASE_STEP;
uint32_t procfile_max_words = PFWORDS_INCREASE_STEP;
size_t procfile_max_allocation = PROCFILE_INCREMENT_BUFFER;

// ----------------------------------------------------------------------------
// An array of words


pfwords *pfwords_add(pfwords *fw, char *str) {
    // debug(D_PROCFILE, PF_PREFIX ":   adding word No %d: '%s'", fw->len, str);

    if(unlikely(fw->len == fw->size)) {
        // debug(D_PROCFILE, PF_PREFIX ":   expanding words");

        fw = reallocz(fw, sizeof(pfwords) + (fw->size + PFWORDS_INCREASE_STEP) * sizeof(char *));
        fw->size += PFWORDS_INCREASE_STEP;
    }

    fw->words[fw->len++] = str;

    return fw;
}

pfwords *pfwords_new(void) {
    // debug(D_PROCFILE, PF_PREFIX ":   initializing words");

    uint32_t size = (procfile_adaptive_initial_allocation) ? procfile_max_words : PFWORDS_INCREASE_STEP;

    pfwords *new = mallocz(sizeof(pfwords) + size * sizeof(char *));
    new->len = 0;
    new->size = size;
    return new;
}

void pfwords_reset(pfwords *fw) {
    // debug(D_PROCFILE, PF_PREFIX ":   reseting words");
    fw->len = 0;
}

void pfwords_free(pfwords *fw) {
    // debug(D_PROCFILE, PF_PREFIX ":   freeing words");

    freez(fw);
}


// ----------------------------------------------------------------------------
// An array of lines

pflines *pflines_add(pflines *fl, uint32_t first_word) {
    // debug(D_PROCFILE, PF_PREFIX ":   adding line %d at word %d", fl->len, first_word);

    if(unlikely(fl->len == fl->size)) {
        // debug(D_PROCFILE, PF_PREFIX ":   expanding lines");

        fl = reallocz(fl, sizeof(pflines) + (fl->size + PFLINES_INCREASE_STEP) * sizeof(ffline));
        fl->size += PFLINES_INCREASE_STEP;
    }

    fl->lines[fl->len].words = 0;
    fl->lines[fl->len++].first = first_word;

    return fl;
}

pflines *pflines_new(void) {
    // debug(D_PROCFILE, PF_PREFIX ":   initializing lines");

    uint32_t size = (unlikely(procfile_adaptive_initial_allocation)) ? procfile_max_words : PFLINES_INCREASE_STEP;

    pflines *new = mallocz(sizeof(pflines) + size * sizeof(ffline));
    new->len = 0;
    new->size = size;
    return new;
}

void pflines_reset(pflines *fl) {
    // debug(D_PROCFILE, PF_PREFIX ":   reseting lines");

    fl->len = 0;
}

void pflines_free(pflines *fl) {
    // debug(D_PROCFILE, PF_PREFIX ":   freeing lines");

    freez(fl);
}


// ----------------------------------------------------------------------------
// The procfile

#define PF_CHAR_IS_SEPARATOR    ' '
#define PF_CHAR_IS_NEWLINE      'N'
#define PF_CHAR_IS_WORD         'W'
#define PF_CHAR_IS_QUOTE        'Q'
#define PF_CHAR_IS_OPEN         'O'
#define PF_CHAR_IS_CLOSE        'C'

void procfile_close(procfile *ff) {
    debug(D_PROCFILE, PF_PREFIX ": Closing file '%s'", ff->filename);

    if(likely(ff->lines)) pflines_free(ff->lines);
    if(likely(ff->words)) pfwords_free(ff->words);

    if(likely(ff->fd != -1)) close(ff->fd);
    freez(ff);
}

procfile *procfile_parser(procfile *ff) {
    debug(D_PROCFILE, PF_PREFIX ": Parsing file '%s'", ff->filename);

    char *s = ff->data, *e = &ff->data[ff->len], *t = ff->data, quote = 0;
    uint32_t l = 0, w = 0;
    int opened = 0;

    ff->lines = pflines_add(ff->lines, w);
    if(unlikely(!ff->lines)) goto cleanup;

    while(likely(s < e)) {
        // we are not at the end

        switch(ff->separators[(uint8_t)(*s)]) {
            case PF_CHAR_IS_OPEN:
                if(s == t) {
                    opened++;
                    t = ++s;
                }
                else if(opened) {
                    opened++;
                    s++;
                }
                else
                    s++;
                continue;

            case PF_CHAR_IS_CLOSE:
                if(opened) {
                    opened--;

                    if(!opened) {
                        *s = '\0';
                        ff->words = pfwords_add(ff->words, t);
                        if(unlikely(!ff->words)) goto cleanup;

                        ff->lines->lines[l].words++;
                        w++;

                        t = ++s;
                    }
                    else
                        s++;
                }
                else
                    s++;
                continue;

            case PF_CHAR_IS_QUOTE:
                if(unlikely(!quote && s == t)) {
                    // quote opened at the beginning
                    quote = *s;
                    t = ++s;
                }
                else if(unlikely(quote && quote == *s)) {
                    // quote closed
                    quote = 0;

                    *s = '\0';
                    ff->words = pfwords_add(ff->words, t);
                    if(unlikely(!ff->words)) goto cleanup;

                    ff->lines->lines[l].words++;
                    w++;

                    t = ++s;
                }
                else
                    s++;
                continue;

            case PF_CHAR_IS_SEPARATOR:
                if(unlikely(quote || opened)) {
                    // we are inside a quote
                    s++;
                    continue;
                }

                if(unlikely(s == t)) {
                    // skip all leading white spaces
                    t = ++s;
                    continue;
                }

                // end of word
                *s = '\0';

                ff->words = pfwords_add(ff->words, t);
                if(unlikely(!ff->words)) goto cleanup;

                ff->lines->lines[l].words++;
                w++;

                t = ++s;
                continue;

            case PF_CHAR_IS_NEWLINE:
                // end of line
                *s = '\0';

                ff->words = pfwords_add(ff->words, t);
                if(unlikely(!ff->words)) goto cleanup;

                ff->lines->lines[l].words++;
                w++;

                // debug(D_PROCFILE, PF_PREFIX ":   ended line %d with %d words", l, ff->lines->lines[l].words);

                ff->lines = pflines_add(ff->lines, w);
                if(unlikely(!ff->lines)) goto cleanup;
                l++;

                t = ++s;
                continue;

            default:
                s++;
                continue;
        }
    }

    if(likely(s > t && t < e)) {
        // the last word
        if(likely(ff->len < ff->size))
            *s = '\0';
        else {
            // we are going to loose the last byte
            ff->data[ff->size - 1] = '\0';
        }

        ff->words = pfwords_add(ff->words, t);
        if(unlikely(!ff->words)) goto cleanup;

        ff->lines->lines[l].words++;
        w++;
    }

    return ff;

cleanup:
    error(PF_PREFIX ": Failed to parse file '%s'", ff->filename);
    procfile_close(ff);
    return NULL;
}

procfile *procfile_readall(procfile *ff) {
    debug(D_PROCFILE, PF_PREFIX ": Reading file '%s'.", ff->filename);

    ssize_t r = 1;
    ff->len = 0;

    while(likely(r > 0)) {
        ssize_t s = ff->len;
        ssize_t x = ff->size - s;

        if(unlikely(!x)) {
            debug(D_PROCFILE, PF_PREFIX ": Expanding data buffer for file '%s'.", ff->filename);

            ff = reallocz(ff, sizeof(procfile) + ff->size + PROCFILE_INCREMENT_BUFFER);
            ff->size += PROCFILE_INCREMENT_BUFFER;
        }

        debug(D_PROCFILE, "Reading file '%s', from position %ld with length %lu", ff->filename, s, ff->size - s);
        r = read(ff->fd, &ff->data[s], ff->size - s);
        if(unlikely(r == -1)) {
            if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) error(PF_PREFIX ": Cannot read from file '%s'", ff->filename);
            procfile_close(ff);
            return NULL;
        }

        ff->len += r;
    }

    debug(D_PROCFILE, "Rewinding file '%s'", ff->filename);
    if(unlikely(lseek(ff->fd, 0, SEEK_SET) == -1)) {
        if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) error(PF_PREFIX ": Cannot rewind on file '%s'.", ff->filename);
        procfile_close(ff);
        return NULL;
    }

    pflines_reset(ff->lines);
    pfwords_reset(ff->words);

    ff = procfile_parser(ff);

    if(unlikely(procfile_adaptive_initial_allocation)) {
        if(unlikely(ff->len > procfile_max_allocation)) procfile_max_allocation = ff->len;
        if(unlikely(ff->lines->len > procfile_max_lines)) procfile_max_lines = ff->lines->len;
        if(unlikely(ff->words->len > procfile_max_words)) procfile_max_words = ff->words->len;
    }

    debug(D_PROCFILE, "File '%s' updated.", ff->filename);
    return ff;
}

static void procfile_set_separators(procfile *ff, const char *separators) {
    static char def[256] = { [0 ... 255] = 0 };

    if(unlikely(!def[255])) {
        // this is thread safe
        // we check that the last byte is non-zero
        // if it is zero, multiple threads may be executing this at the same time
        // setting in def[] the exact same values
        int i;
        for(i = 0; likely(i < 256) ;i++) {
            if(unlikely(i == '\n' || i == '\r')) def[i] = PF_CHAR_IS_NEWLINE;
            else if(unlikely(isspace(i) || !isprint(i))) def[i] = PF_CHAR_IS_SEPARATOR;
            else def[i] = PF_CHAR_IS_WORD;
        }
    }

    // copy the default
    char *ffs = ff->separators, *ffd = def, *ffe = &def[256];
    while(likely(ffd != ffe)) *ffs++ = *ffd++;

    // set the separators
    if(unlikely(!separators))
        separators = " \t=|";

    ffs = ff->separators;
    const char *s = separators;
    while(likely(*s))
        ffs[(int)*s++] = PF_CHAR_IS_SEPARATOR;
}

void procfile_set_quotes(procfile *ff, const char *quotes) {
    // remove all quotes
    int i;
    for(i = 0; i < 256 ; i++)
        if(unlikely(ff->separators[i] == PF_CHAR_IS_QUOTE))
            ff->separators[i] = PF_CHAR_IS_WORD;

    // if nothing given, return
    if(unlikely(!quotes || !*quotes))
        return;

    // set the quotes
    char *ffs = ff->separators;
    const char *s = quotes;
    while(likely(*s))
        ffs[(int)*s++] = PF_CHAR_IS_QUOTE;
}

void procfile_set_open_close(procfile *ff, const char *open, const char *close) {
    // remove all open/close
    int i;
    for(i = 0; i < 256 ; i++)
        if(unlikely(ff->separators[i] == PF_CHAR_IS_OPEN || ff->separators[i] == PF_CHAR_IS_CLOSE))
            ff->separators[i] = PF_CHAR_IS_WORD;

    // if nothing given, return
    if(unlikely(!open || !*open || !close || !*close))
        return;

    // set the openings
    char *ffs = ff->separators;
    const char *s = open;
    while(likely(*s))
        ffs[(int)*s++] = PF_CHAR_IS_OPEN;

    s = close;
    while(likely(*s))
        ffs[(int)*s++] = PF_CHAR_IS_CLOSE;
}

procfile *procfile_open(const char *filename, const char *separators, uint32_t flags) {
    debug(D_PROCFILE, PF_PREFIX ": Opening file '%s'", filename);

    int fd = open(filename, O_RDONLY, 0666);
    if(unlikely(fd == -1)) {
        if(unlikely(!(flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) error(PF_PREFIX ": Cannot open file '%s'", filename);
        return NULL;
    }

    size_t size = (unlikely(procfile_adaptive_initial_allocation)) ? procfile_max_allocation : PROCFILE_INCREMENT_BUFFER;
    procfile *ff = mallocz(sizeof(procfile) + size);
    strncpyz(ff->filename, filename, FILENAME_MAX);

    ff->fd = fd;
    ff->size = size;
    ff->len = 0;
    ff->flags = flags;

    ff->lines = pflines_new();
    ff->words = pfwords_new();

    if(unlikely(!ff->lines || !ff->words)) {
        error(PF_PREFIX ": Cannot initialize parser for file '%s'", filename);
        procfile_close(ff);
        return NULL;
    }

    procfile_set_separators(ff, separators);

    debug(D_PROCFILE, "File '%s' opened.", filename);
    return ff;
}

procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators, uint32_t flags) {
    if(unlikely(!ff)) return procfile_open(filename, separators, flags);

    if(likely(ff->fd != -1)) close(ff->fd);

    ff->fd = open(filename, O_RDONLY, 0666);
    if(unlikely(ff->fd == -1)) {
        procfile_close(ff);
        return NULL;
    }

    strncpyz(ff->filename, filename, FILENAME_MAX);

    ff->flags = flags;

    // do not do the separators again if NULL is given
    if(likely(separators)) procfile_set_separators(ff, separators);

    return ff;
}

// ----------------------------------------------------------------------------
// example parsing of procfile data

void procfile_print(procfile *ff) {
    uint32_t lines = procfile_lines(ff), l;
    char *s;

    debug(D_PROCFILE, "File '%s' with %u lines and %u words", ff->filename, ff->lines->len, ff->words->len);

    for(l = 0; likely(l < lines) ;l++) {
        uint32_t words = procfile_linewords(ff, l);

        debug(D_PROCFILE, " line %u starts at word %u and has %u words", l, ff->lines->lines[l].first, ff->lines->lines[l].words);

        uint32_t w;
        for(w = 0; likely(w < words) ;w++) {
            s = procfile_lineword(ff, l, w);
            debug(D_PROCFILE, "     [%u.%u] '%s'", l, w, s);
        }
    }
}
