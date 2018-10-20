/* SPDX-License-Identifier: GPL-3.0-or-later */
#include "config.h"
#include "common.h"
#include "clocks.h"

void netdata_cleanup_and_exit(int ret) {
	exit(ret);
}

#define PF_PREFIX "PROCFILE"
#define PFWORDS_INCREASE_STEP 200
#define PFLINES_INCREASE_STEP 10
#define PROCFILE_INCREMENT_BUFFER 512
extern size_t procfile_max_lines;
extern size_t procfile_max_words;
extern size_t procfile_max_allocation;


static inline void pflines_reset(pflines *fl) {
    // debug(D_PROCFILE, PF_PREFIX ":   reseting lines");

    fl->len = 0;
}

static inline void pflines_free(pflines *fl) {
    // debug(D_PROCFILE, PF_PREFIX ":   freeing lines");

    freez(fl);
}

static inline void pfwords_reset(pfwords *fw) {
    // debug(D_PROCFILE, PF_PREFIX ":   reseting words");
    fw->len = 0;
}


static inline void pfwords_add(procfile *ff, char *str) {
    // debug(D_PROCFILE, PF_PREFIX ":   adding word No %d: '%s'", fw->len, str);

    pfwords *fw = ff->words;
    if(unlikely(fw->len == fw->size)) {
        // debug(D_PROCFILE, PF_PREFIX ":   expanding words");

        ff->words = fw = reallocz(fw, sizeof(pfwords) + (fw->size + PFWORDS_INCREASE_STEP) * sizeof(char *));
        fw->size += PFWORDS_INCREASE_STEP;
    }

    fw->words[fw->len++] = str;
}

NEVERNULL
static inline size_t *pflines_add(procfile *ff) {
    // debug(D_PROCFILE, PF_PREFIX ":   adding line %d at word %d", fl->len, first_word);

    pflines *fl = ff->lines;
    if(unlikely(fl->len == fl->size)) {
        // debug(D_PROCFILE, PF_PREFIX ":   expanding lines");

        ff->lines = fl = reallocz(fl, sizeof(pflines) + (fl->size + PFLINES_INCREASE_STEP) * sizeof(ffline));
        fl->size += PFLINES_INCREASE_STEP;
    }

    ffline *ffl = &fl->lines[fl->len++];
    ffl->words = 0;
    ffl->first = ff->words->len;

    return &ffl->words;
}


NOINLINE
static void procfile_parser(procfile *ff) {
    // debug(D_PROCFILE, PF_PREFIX ": Parsing file '%s'", ff->filename);

    char  *s = ff->data                 // our current position
        , *e = &ff->data[ff->len]       // the terminating null
        , *t = ff->data;                // the first character of a word (or quoted / parenthesized string)

                                        // the look up array to find our type of character
    PF_CHAR_TYPE *separators = ff->separators;

    char quote = 0;                     // the quote character - only when in quoted string
    size_t opened = 0;                  // counts the number of open parenthesis

    size_t *line_words = pflines_add(ff);

    while(s < e) {
        PF_CHAR_TYPE ct = separators[(unsigned char)(*s)];

        // this is faster than a switch()
        // read more here: http://lazarenko.me/switch/
        switch(ct) {
        	case PF_CHAR_IS_SEPARATOR:
	            if(!quote && !opened) {
	                if (s != t) {
	                    // separator, but we have word before it
	                    *s = '\0';
	                    pfwords_add(ff, t);
	                    (*line_words)++;
	                }
	                t = s + 1;
	            }
	            // fallthrough

			case PF_CHAR_IS_WORD:
				s++;
				break;


		    case PF_CHAR_IS_NEWLINE:
		        // end of line

		        *s = '\0';
		        pfwords_add(ff, t);
		        (*line_words)++;
		        t = ++s;

		        // debug(D_PROCFILE, PF_PREFIX ":   ended line %d with %d words", l, ff->lines->lines[l].words);

		        line_words = pflines_add(ff);
		    	break;

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
		            pfwords_add(ff, t);
		            (*line_words)++;
		            t = ++s;
		        }
		        else
		            s++;
		    	break;

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
		    	break;

		    case PF_CHAR_IS_CLOSE:
		        if(opened) {
		            opened--;

		            if(!opened) {
		                *s = '\0';
		                pfwords_add(ff, t);
		                (*line_words)++;
		                t = ++s;
		            }
		            else
		                s++;
		        }
		        else
		            s++;
		    	break;

		    default:
		        fatal("Internal Error: procfile_readall() does not handle all the cases.");
    	}
	}

    if(likely(s > t && t < e)) {
        // the last word
        if(unlikely(ff->len >= ff->size)) {
            // we are going to loose the last byte
            s = &ff->data[ff->size - 1];
        }

        *s = '\0';
        pfwords_add(ff, t);
        (*line_words)++;
        // t = ++s;
    }
}


procfile *procfile_readall1(procfile *ff) {
    // debug(D_PROCFILE, PF_PREFIX ": Reading file '%s'.", ff->filename);

    ff->len = 0;    // zero the used size
    ssize_t r = 1;  // read at least once
    while(r > 0) {
        ssize_t s = ff->len;
        ssize_t x = ff->size - s;

        if(unlikely(!x)) {
            debug(D_PROCFILE, PF_PREFIX ": Expanding data buffer for file '%s'.", procfile_filename(ff));
            ff = reallocz(ff, sizeof(procfile) + ff->size + PROCFILE_INCREMENT_BUFFER);
            ff->size += PROCFILE_INCREMENT_BUFFER;
        }

        debug(D_PROCFILE, "Reading file '%s', from position %zd with length %zd", procfile_filename(ff), s, (ssize_t)(ff->size - s));
        r = read(ff->fd, &ff->data[s], ff->size - s);
        if(unlikely(r == -1)) {
            if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) error(PF_PREFIX ": Cannot read from file '%s' on fd %d", procfile_filename(ff), ff->fd);
            procfile_close(ff);
            return NULL;
        }

        ff->len += r;
    }

    // debug(D_PROCFILE, "Rewinding file '%s'", ff->filename);
    if(unlikely(lseek(ff->fd, 0, SEEK_SET) == -1)) {
        if(unlikely(!(ff->flags & PROCFILE_FLAG_NO_ERROR_ON_FILE_IO))) error(PF_PREFIX ": Cannot rewind on file '%s'.", procfile_filename(ff));
        procfile_close(ff);
        return NULL;
    }

    pflines_reset(ff->lines);
    pfwords_reset(ff->words);
    procfile_parser(ff);

    if(unlikely(procfile_adaptive_initial_allocation)) {
        if(unlikely(ff->len > procfile_max_allocation)) procfile_max_allocation = ff->len;
        if(unlikely(ff->lines->len > procfile_max_lines)) procfile_max_lines = ff->lines->len;
        if(unlikely(ff->words->len > procfile_max_words)) procfile_max_words = ff->words->len;
    }

    // debug(D_PROCFILE, "File '%s' updated.", ff->filename);
    return ff;
}








// ==============
// --- Poor man cycle counting.
static unsigned long tsc;

void begin_tsc(void)
{
  unsigned long a, d;
  asm volatile ("cpuid\nrdtsc" : "=a" (a), "=d" (d) : "0" (0) : "ebx", "ecx");
  tsc = ((unsigned long)d << 32) | (unsigned long)a;
}

unsigned long end_tsc(void)
{
  unsigned long a, d;
  asm volatile ("rdtscp" : "=a" (a), "=d" (d) : : "ecx");
  return (((unsigned long)d << 32) | (unsigned long)a) - tsc;
}
// ==============


unsigned long test_netdata_internal(void) {
	static procfile *ff = NULL;

	ff = procfile_reopen(ff, "/proc/self/status", " \t:,-()/", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) {
		fprintf(stderr, "Failed to open filename\n");
		exit(1);
	}

	begin_tsc();
	ff = procfile_readall(ff);
	unsigned long c = end_tsc();

	if(!ff) {
		fprintf(stderr, "Failed to read filename\n");
		exit(1);
	}

	return c;
}

unsigned long test_method1(void) {
	static procfile *ff = NULL;

	ff = procfile_reopen(ff, "/proc/self/status", " \t:,-()/", PROCFILE_FLAG_NO_ERROR_ON_FILE_IO);
	if(!ff) {
		fprintf(stderr, "Failed to open filename\n");
		exit(1);
	}

	begin_tsc();
	ff = procfile_readall1(ff);
	unsigned long c = end_tsc();

	if(!ff) {
		fprintf(stderr, "Failed to read filename\n");
		exit(1);
	}

	return c;
}

//--- Test
int main(int argc, char **argv)
{
	(void)argc; (void)argv;

	int i, max = 1000000;

	unsigned long c1 = 0;
	test_netdata_internal();
	for(i = 0; i < max ; i++)
		c1 += test_netdata_internal();

	unsigned long c2 = 0;
	test_method1();
	for(i = 0; i < max ; i++)
		c2 += test_method1();

	printf("netdata internal: completed in %lu cycles, %lu cycles per read, %0.2f %%.\n", c1, c1 / max, (float)c1 * 100.0 / (float)c1);
	printf("method1         : completed in %lu cycles, %lu cycles per read, %0.2f %%.\n", c2, c2 / max, (float)c2 * 100.0 / (float)c1);

	return 0;
}
