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

#include "log.h"
#include "procfile.h"

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
	// debug(D_PROCFILE, PF_PREFIX ":	adding word No %d: '%s'", fw->len, str);

	if(fw->len == fw->size) {
		// debug(D_PROCFILE, PF_PREFIX ":	expanding words");

		pfwords *new = realloc(fw, sizeof(pfwords) + (fw->size + PFWORDS_INCREASE_STEP) * sizeof(char *));
		if(!new) {
			error(PF_PREFIX ":	failed to expand words");
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
	// debug(D_PROCFILE, PF_PREFIX ":	initializing words");

	uint32_t size = (procfile_adaptive_initial_allocation) ? procfile_max_words : PFWORDS_INCREASE_STEP;

	pfwords *new = malloc(sizeof(pfwords) + size * sizeof(char *));
	if(!new) return NULL;

	new->len = 0;
	new->size = size;
	return new;
}

void pfwords_reset(pfwords *fw) {
	// debug(D_PROCFILE, PF_PREFIX ":	reseting words");
	fw->len = 0;
}

void pfwords_free(pfwords *fw) {
	// debug(D_PROCFILE, PF_PREFIX ":	freeing words");

	free(fw);
}


// ----------------------------------------------------------------------------
// An array of lines

pflines *pflines_add(pflines *fl, uint32_t first_word) {
	// debug(D_PROCFILE, PF_PREFIX ":	adding line %d at word %d", fl->len, first_word);

	if(fl->len == fl->size) {
		// debug(D_PROCFILE, PF_PREFIX ":	expanding lines");

		pflines *new = realloc(fl, sizeof(pflines) + (fl->size + PFLINES_INCREASE_STEP) * sizeof(ffline));
		if(!new) {
			error(PF_PREFIX ":	failed to expand lines");
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
	// debug(D_PROCFILE, PF_PREFIX ":	initializing lines");

	uint32_t size = (procfile_adaptive_initial_allocation) ? procfile_max_words : PFLINES_INCREASE_STEP;

	pflines *new = malloc(sizeof(pflines) + size * sizeof(ffline));
	if(!new) return NULL;

	new->len = 0;
	new->size = size;
	return new;
}

void pflines_reset(pflines *fl) {
	// debug(D_PROCFILE, PF_PREFIX ":	reseting lines");

	fl->len = 0;
}

void pflines_free(pflines *fl) {
	// debug(D_PROCFILE, PF_PREFIX ":	freeing lines");

	free(fl);
}


// ----------------------------------------------------------------------------
// The procfile

#define PF_CHAR_IS_SEPARATOR	0
#define PF_CHAR_IS_NEWLINE	1
#define PF_CHAR_IS_WORD		2

void procfile_close(procfile *ff) {
	debug(D_PROCFILE, PF_PREFIX ": Closing file '%s'", ff->filename);

	if(ff->lines) pflines_free(ff->lines);
	if(ff->words) pfwords_free(ff->words);

	if(ff->fd != -1) close(ff->fd);
	free(ff);
}

procfile *procfile_parser(procfile *ff) {
	debug(D_PROCFILE, PF_PREFIX ": Parsing file '%s'", ff->filename);

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

				// debug(D_PROCFILE, PF_PREFIX ":	ended line %d with %d words", l, ff->lines->lines[l].words);

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
	error(PF_PREFIX ": Failed to parse file '%s'. Reason: %s", ff->filename, strerror(errno));
	procfile_close(ff);
	return NULL;
}

procfile *procfile_readall(procfile *ff) {
	debug(D_PROCFILE, PF_PREFIX ": Reading file '%s'.", ff->filename);

	ssize_t s = 0, r = ff->size, x = ff->size;
	ff->len = 0;

	while(r == x) {
		if(s) {
			debug(D_PROCFILE, PF_PREFIX ": Expanding data buffer for file '%s'.", ff->filename);

			procfile *new = realloc(ff, sizeof(procfile) + ff->size + PROCFILE_INCREMENT_BUFFER);
			if(!new) {
				error(PF_PREFIX ": Cannot allocate memory for file '%s'. Reason: %s", ff->filename, strerror(errno));
				procfile_close(ff);
				return NULL;
			}
			ff = new;
			ff->size += PROCFILE_INCREMENT_BUFFER;
			x = PROCFILE_INCREMENT_BUFFER;
		}

		debug(D_PROCFILE, "Reading file '%s', from position %ld with length %ld", ff->filename, s, ff->size - s);
		r = read(ff->fd, &ff->data[s], ff->size - s);
		if(r == -1) {
			error(PF_PREFIX ": Cannot read from file '%s'. Reason: %s", ff->filename, strerror(errno));
			procfile_close(ff);
			return NULL;
		}

		ff->len += r;
		s = ff->len;
	}

	debug(D_PROCFILE, "Rewinding file '%s'", ff->filename);
	if(lseek(ff->fd, 0, SEEK_SET) == -1) {
		error(PF_PREFIX ": Cannot rewind on file '%s'.", ff->filename);
		procfile_close(ff);
		return NULL;
	}

	pflines_reset(ff->lines);
	pfwords_reset(ff->words);

	ff = procfile_parser(ff);

	if(procfile_adaptive_initial_allocation) {
		if(ff->len > procfile_max_allocation) procfile_max_allocation = ff->len;
		if(ff->lines->len > procfile_max_lines) procfile_max_lines = ff->lines->len;
		if(ff->words->len > procfile_max_words) procfile_max_words = ff->words->len;
	}

	debug(D_PROCFILE, "File '%s' updated.", ff->filename);
	return ff;
}

procfile *procfile_open(const char *filename, const char *separators) {
	debug(D_PROCFILE, PF_PREFIX ": Opening file '%s'", filename);

	int fd = open(filename, O_RDONLY, 0666);
	if(fd == -1) {
		error(PF_PREFIX ": Cannot open file '%s'. Reason: %s", filename, strerror(errno));
		return NULL;
	}

	size_t size = (procfile_adaptive_initial_allocation) ? procfile_max_allocation : PROCFILE_INCREMENT_BUFFER;
	procfile *ff = malloc(sizeof(procfile) + size);
	if(!ff) {
		error(PF_PREFIX ": Cannot allocate memory for file '%s'. Reason: %s", filename, strerror(errno));
		close(fd);
		return NULL;
	}

	strncpy(ff->filename, filename, FILENAME_MAX);
	ff->filename[FILENAME_MAX] = '\0';

	ff->fd = fd;
	ff->size = size;
	ff->len = 0;

	ff->lines = pflines_new();
	ff->words = pfwords_new();

	if(!ff->lines || !ff->words) {
		error(PF_PREFIX ": Cannot initialize parser for file '%s'. Reason: %s", filename, strerror(errno));
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

	debug(D_PROCFILE, "File '%s' opened.", filename);
	return ff;
}

procfile *procfile_reopen(procfile *ff, const char *filename, const char *separators) {
	if(!ff) return procfile_open(filename, separators);

	if(ff->fd != -1) close(ff->fd);

	ff->fd = open(filename, O_RDONLY, 0666);
	if(ff->fd == -1) {
		procfile_close(ff);
		return NULL;
	}

	strncpy(ff->filename, filename, FILENAME_MAX);
	ff->filename[FILENAME_MAX] = '\0';

	return ff;
}

// ----------------------------------------------------------------------------
// example parsing of procfile data

void procfile_print(procfile *ff) {
	uint32_t lines = procfile_lines(ff), l;
	uint32_t words, w;
	char *s;

	debug(D_PROCFILE, "File '%s' with %d lines and %d words", ff->filename, ff->lines->len, ff->words->len);

	for(l = 0; l < lines ;l++) {
		words = procfile_linewords(ff, l);

		debug(D_PROCFILE, "	line %d starts at word %d and has %d words", l, ff->lines->lines[l].first, ff->lines->lines[l].words);

		for(w = 0; w < words ;w++) {
			s = procfile_lineword(ff, l, w);
			debug(D_PROCFILE, "		[%d.%d] '%s'", l, w, s);
		}
	}
}
