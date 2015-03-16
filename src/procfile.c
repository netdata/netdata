#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>
#include <fcntl.h>
#include <string.h>
#include <malloc.h>
#include <inttypes.h>
#include <ctype.h>
#include <time.h>
#include <sys/time.h>
#include <sys/wait.h>

#include <sys/types.h>
#include <sys/stat.h>
#include <sys/mman.h>

#include "procfile.h"

#define PF_PREFIX "PROCFILE"
int pfdebug = 0;

// ----------------------------------------------------------------------------
// An array of words

#define PFWORDS_INCREASE_STEP 200

pfwords *pfwords_add(pfwords *fw, char *str) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	adding word No %d: '%s'\n", fw->len, str);

	if(fw->len == fw->size) {
		if(pfdebug) fprintf(stderr, PF_PREFIX ":	expanding words\n");

		pfwords *new = realloc(fw, sizeof(pfwords) + (fw->size + PFWORDS_INCREASE_STEP) * sizeof(char *));
		if(!new) {
			fprintf(stderr, PF_PREFIX ":	failed to expand words\n");
			free(fw);
			return NULL;
		}
		fw = new;
		fw->size += PFWORDS_INCREASE_STEP;
	}

	fw->words[fw->len++] = str;

	return fw;
}

pfwords *pfwords_new(void) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	initializing words\n");

	pfwords *new = malloc(sizeof(pfwords) + PFWORDS_INCREASE_STEP * sizeof(char *));
	if(!new) return NULL;

	new->len = 0;
	new->size = PFWORDS_INCREASE_STEP;
	return new;
}

void pfwords_reset(pfwords *fw) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	reseting words\n");
	fw->len = 0;
}

void pfwords_free(pfwords *fw) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	freeing words\n");

	free(fw);
}


// ----------------------------------------------------------------------------
// An array of lines

#define PFLINES_INCREASE_STEP 10

pflines *pflines_add(pflines *fl, uint32_t first_word) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	adding line %d at word %d\n", fl->len, first_word);

	if(fl->len == fl->size) {
		if(pfdebug) fprintf(stderr, PF_PREFIX ":	expanding lines\n");

		pflines *new = realloc(fl, sizeof(pflines) + (fl->size + PFLINES_INCREASE_STEP) * sizeof(ffline));
		if(!new) {
			fprintf(stderr, PF_PREFIX ":	failed to expand lines\n");
			free(fl);
			return NULL;
		}
		fl = new;
		fl->size += PFLINES_INCREASE_STEP;
	}

	fl->lines[fl->len].words = 0;
	fl->lines[fl->len++].first = first_word;

	return fl;
}

pflines *pflines_new(void) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	initializing lines\n");

	pflines *new = malloc(sizeof(pflines) + PFLINES_INCREASE_STEP * sizeof(ffline));
	if(!new) return NULL;

	new->len = 0;
	new->size = PFLINES_INCREASE_STEP;
	return new;
}

void pflines_reset(pflines *fl) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	reseting lines\n");

	fl->len = 0;
}

void pflines_free(pflines *fl) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ":	freeing lines\n");

	free(fl);
}


// ----------------------------------------------------------------------------
// The procfile

#define PROCFILE_INITIAL_BUFFER 256
#define PROCFILE_INCREMENT_BUFFER 512

#define PF_CHAR_IS_SEPARATOR	0
#define PF_CHAR_IS_NEWLINE	1
#define PF_CHAR_IS_WORD		2

void procfile_close(procfile *ff) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ": Closing file '%s'. Reason: %s\n", ff->filename, strerror(errno));

	if(ff->lines) pflines_free(ff->lines);
	if(ff->words) pfwords_free(ff->words);

	if(ff->fd != -1) close(ff->fd);
	free(ff);
}

procfile *procfile_parser(procfile *ff) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ": Parsing file '%s'\n", ff->filename);

	char *s = ff->data, *e = ff->data, *t = ff->data;
	uint32_t l = 0, w = 0;
	e += ff->len;
	
	ff->lines = pflines_add(ff->lines, w);
	if(!ff->lines) goto cleanup;

	while(s < e) {
		switch(ff->separators[(int)(*s)]) {
			case PF_CHAR_IS_SEPARATOR:
				if(s == t) {
					// skip all leading white spaces
					t = ++s;
					continue;
				}

				// end of word
				*s = '\0';

				ff->words = pfwords_add(ff->words, t);
				if(!ff->words) goto cleanup;

				ff->lines->lines[l].words++;
				w++;

				t = ++s;
				continue;

			case PF_CHAR_IS_NEWLINE:
				// end of line
				*s = '\0';

				ff->words = pfwords_add(ff->words, t);
				if(!ff->words) goto cleanup;

				ff->lines->lines[l].words++;
				w++;

				if(pfdebug) fprintf(stderr, PF_PREFIX ":	ended line %d with %d words\n", l, ff->lines->lines[l].words);

				ff->lines = pflines_add(ff->lines, w);
				if(!ff->lines) goto cleanup;
				l++;

				t = ++s;
				continue;

			default:
				s++;
				continue;
		}
	}

	if(s != t) {
		// the last word
		if(ff->len < ff->size) *s = '\0';
		else {
			// we are going to loose the last byte
			ff->data[ff->size - 1] = '\0';
		}

		ff->words = pfwords_add(ff->words, t);
		if(!ff->words) goto cleanup;

		ff->lines->lines[l].words++;
		w++;
	}

	return ff;

cleanup:
	fprintf(stderr, PF_PREFIX ": Failed to parse file '%s'. Reason: %s\n", ff->filename, strerror(errno));
	procfile_close(ff);
	return NULL;
}

procfile *procfile_readall(procfile *ff) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ": Reading file '%s'.\n", ff->filename);

	ssize_t s = 0, r = ff->size, x = ff->size;
	ff->len = 0;

	while(r == x) {
		if(s) {
			if(pfdebug) fprintf(stderr, PF_PREFIX ": Expanding data buffer for file '%s'.\n", ff->filename);

			procfile *new = realloc(ff, sizeof(procfile) + ff->size + PROCFILE_INCREMENT_BUFFER);
			if(!new) {
				fprintf(stderr, PF_PREFIX ": Cannot allocate memory for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
				procfile_close(ff);
				return NULL;
			}
			ff = new;
			ff->size += PROCFILE_INCREMENT_BUFFER;
			x = PROCFILE_INCREMENT_BUFFER;
		}

		r = read(ff->fd, &ff->data[s], ff->size - s);
		if(r == -1) {
			fprintf(stderr, PF_PREFIX ": Cannot read from file '%s'. Reason: %s\n", ff->filename, strerror(errno));
			procfile_close(ff);
			return NULL;
		}

		ff->len += r;
		s = ff->len;
	}

	if(lseek(ff->fd, 0, SEEK_SET) == -1) {
		fprintf(stderr, PF_PREFIX ": Cannot rewind on file '%s'.\n", ff->filename);
		procfile_close(ff);
		return NULL;
	}

	pflines_reset(ff->lines);
	pfwords_reset(ff->words);

	ff = procfile_parser(ff);

	return ff;
}

procfile *procfile_open(const char *filename, const char *separators) {
	if(pfdebug) fprintf(stderr, PF_PREFIX ": Opening file '%s'\n", filename);

	int fd = open(filename, O_RDONLY, 0666);
	if(fd == -1) {
		fprintf(stderr, PF_PREFIX ": Cannot open file '%s'. Reason: %s\n", filename, strerror(errno));
		return NULL;
	}

	procfile *ff = malloc(sizeof(procfile) + PROCFILE_INITIAL_BUFFER);
	if(!ff) {
		fprintf(stderr, PF_PREFIX ": Cannot allocate memory for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
		close(fd);
		return NULL;
	}

	strncpy(ff->filename, filename, FILENAME_MAX);
	ff->filename[FILENAME_MAX] = '\0';

	ff->fd = fd;
	ff->size = PROCFILE_INITIAL_BUFFER;
	ff->len = 0;

	ff->lines = pflines_new();
	ff->words = pfwords_new();

	if(!ff->lines || !ff->words) {
		fprintf(stderr, PF_PREFIX ": Cannot initialize parser for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
		procfile_close(ff);
		return NULL;
	}

	int i;
	for(i = 0; i < 256 ;i++) {
		if(i == '\n' || i == '\r') ff->separators[i] = PF_CHAR_IS_NEWLINE;
		else if(isspace(i) || !isprint(i)) ff->separators[i] = PF_CHAR_IS_SEPARATOR;
		else ff->separators[i] = PF_CHAR_IS_WORD;
	}

	if(!separators) separators = " \t=|";
	const char *s = separators;
	while(*s) ff->separators[(int)*s++] = PF_CHAR_IS_SEPARATOR;

	return ff;
}


// ----------------------------------------------------------------------------
// example parsing of procfile data

void procfile_print(procfile *ff) {
	uint32_t lines = procfile_lines(ff), l;
	uint32_t words, w;
	char *s;

	fprintf(stderr, "File '%s' with %d lines and %d words\n", ff->filename, ff->lines->len, ff->words->len);

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);

		fprintf(stderr, "	line %d starts at word %d and has %d words\n", l, ff->lines->lines[l].first, ff->lines->lines[l].words);

		for(w = 0; w < words ;w++) {
			s = procfile_lineword(ff, l, w);
			fprintf(stderr, "		[%d.%d] '%s'\n", l, w, s);
		}
	}
}
