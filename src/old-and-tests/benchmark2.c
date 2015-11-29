#define _GNU_SOURCE

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

/*
 * This is a library for reading kernel files from /proc
 *
 * The idea is this:
 *
 *  - every file is opened once.
 *  - to read updated contents, we rewind it (lseek() to 0) and read again.
 *  - for every file, we use a buffer that is adjusted to fit its entire
 *    contents in memory. This allows us to read it with a single read() call.
 *    (this provides atomicity / consistency on the data read from the kernel)
 *  - once the data are read, we update two arrays of pointers:
 *     - a words array, pointing to each word in the data read
 *     - a lines array, pointing to the first word for each line
 *    This is highly optimized. Both arrays are automatically adjusted to
 *    fit all contents and they are updated in a single pass on the data:
 *     - a raspberry Pi can process 5.000+ files / sec.
 *     - a J1900 celeron processor can process 23.000+ files / sec.
*/

#define PLUGIN_NAME "proc.plugin"

int debug = 0;
int update_every = 1;

unsigned long long usecdiff(struct timeval *now, struct timeval *last) {
		return ((((now->tv_sec * 1000000ULL) + now->tv_usec) - ((last->tv_sec * 1000000ULL) + last->tv_usec)));
}

// ----------------------------------------------------------------------------
// An array of words

typedef struct {
	uint32_t len;	// used entries
	uint32_t size;	// capacity
	char *words[];	// array of pointers
} ffwords;

#define FFWORDS_INCREASE_STEP 200
#define ffwords_get(fw, i) ((i < fw->size)?fw->words[i]:NULL)

ffwords *ffwords_add(ffwords *fw, char *str) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	adding word No %d: '%s'\n", fw->len, str);

	if(fw->len == fw->size) {
		if(debug) fprintf(stderr, PLUGIN_NAME ":	expanding words\n");

		ffwords *new = realloc(fw, sizeof(ffwords) + (fw->size + FFWORDS_INCREASE_STEP) * sizeof(char *));
		if(!new) {
			fprintf(stderr, PLUGIN_NAME ":	failed to expand words\n");
			free(fw);
			return NULL;
		}
		fw = new;
		fw->size += FFWORDS_INCREASE_STEP;
	}

	fw->words[fw->len++] = str;

	return fw;
}

ffwords *ffwords_new(void) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	initializing words\n");

	ffwords *new = malloc(sizeof(ffwords) + FFWORDS_INCREASE_STEP * sizeof(char *));
	if(!new) return NULL;

	new->len = 0;
	new->size = FFWORDS_INCREASE_STEP;
	return new;
}

void ffwords_reset(ffwords *fw) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	reseting words\n");
	fw->len = 0;
}

void ffwords_free(ffwords *fw) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	freeing words\n");

	free(fw);
}


// ----------------------------------------------------------------------------
// An array of lines

typedef struct {
	uint32_t words;		// how many words this line has
	uint32_t first;		// the id of the first word of this line
				// in the words array
} ffline;

typedef struct {
	uint32_t len;		// used entries
	uint32_t size;		// capacity
	ffline lines[];		// array of lines
} fflines;

#define FFLINES_INCREASE_STEP 10

fflines *fflines_add(fflines *fl, uint32_t first_word) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	adding line %d at word %d\n", fl->len, first_word);

	if(fl->len == fl->size) {
		if(debug) fprintf(stderr, PLUGIN_NAME ":	expanding lines\n");

		fflines *new = realloc(fl, sizeof(fflines) + (fl->size + FFLINES_INCREASE_STEP) * sizeof(ffline));
		if(!new) {
			fprintf(stderr, PLUGIN_NAME ":	failed to expand lines\n");
			free(fl);
			return NULL;
		}
		fl = new;
		fl->size += FFLINES_INCREASE_STEP;
	}

	fl->lines[fl->len].words = 0;
	fl->lines[fl->len++].first = first_word;

	return fl;
}

fflines *fflines_new(void) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	initializing lines\n");

	fflines *new = malloc(sizeof(fflines) + FFLINES_INCREASE_STEP * sizeof(ffline));
	if(!new) return NULL;

	new->len = 0;
	new->size = FFLINES_INCREASE_STEP;
	return new;
}

void fflines_reset(fflines *fl) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	reseting lines\n");

	fl->len = 0;
}

void fflines_free(fflines *fl) {
	if(debug) fprintf(stderr, PLUGIN_NAME ":	freeing lines\n");

	free(fl);
}


// ----------------------------------------------------------------------------
// The fastfile

typedef struct {
	char filename[FILENAME_MAX + 1];
	int fd;			// the file desriptor
	ssize_t len;		// the bytes we have placed into data
	ssize_t size;		// the bytes we have allocated for data
	fflines *lines;
	ffwords *words;
	char separators[256];
	char data[];		// allocated buffer to keep file contents
} fastfile;

#define FASTFILE_INITIAL_BUFFER 512
#define FASTFILE_INCREMENT_BUFFER 1024

#define CHAR_IS_SEPARATOR	0
#define CHAR_IS_NEWLINE		1
#define CHAR_IS_WORD		2

void fastfile_close(fastfile *ff) {
	if(debug) fprintf(stderr, PLUGIN_NAME ": Closing file '%s'. Reason: %s\n", ff->filename, strerror(errno));

	if(ff->lines) fflines_free(ff->lines);
	if(ff->words) ffwords_free(ff->words);

	if(ff->fd != -1) close(ff->fd);
	free(ff);
}

fastfile *fastfile_parser(fastfile *ff) {
	if(debug) fprintf(stderr, PLUGIN_NAME ": Parsing file '%s'\n", ff->filename);

	char *s = ff->data, *e = ff->data, *t = ff->data;
	uint32_t l = 0, w = 0;
	e += ff->len;

	ff->lines = fflines_add(ff->lines, w);
	if(!ff->lines) goto cleanup;

	while(s < e) {
		switch(ff->separators[(int)(*s)]) {
			case CHAR_IS_SEPARATOR:
				if(s == t) {
					// skip all leading white spaces
					t = ++s;
					continue;
				}

				// end of word
				*s = '\0';

				ff->words = ffwords_add(ff->words, t);
				if(!ff->words) goto cleanup;

				ff->lines->lines[l].words++;
				w++;

				t = ++s;
				continue;

			case CHAR_IS_NEWLINE:
				// end of line
				*s = '\0';

				ff->words = ffwords_add(ff->words, t);
				if(!ff->words) goto cleanup;

				ff->lines->lines[l].words++;
				w++;

				if(debug) fprintf(stderr, PLUGIN_NAME ":	ended line %d with %d words\n", l, ff->lines->lines[l].words);

				ff->lines = fflines_add(ff->lines, w);
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

		ff->words = ffwords_add(ff->words, t);
		if(!ff->words) goto cleanup;

		ff->lines->lines[l].words++;
		w++;
	}

	return ff;

cleanup:
	fprintf(stderr, PLUGIN_NAME ": Failed to parse file '%s'. Reason: %s\n", ff->filename, strerror(errno));
	fastfile_close(ff);
	return NULL;
}

fastfile *fastfile_readall(fastfile *ff) {
	if(debug) fprintf(stderr, PLUGIN_NAME ": Reading file '%s'.\n", ff->filename);

	ssize_t s = 0, r = ff->size, x = ff->size;
	ff->len = 0;

	while(r == x) {
		if(s) {
			if(debug) fprintf(stderr, PLUGIN_NAME ": Expanding data buffer for file '%s'.\n", ff->filename);

			fastfile *new = realloc(ff, sizeof(fastfile) + ff->size + FASTFILE_INCREMENT_BUFFER);
			if(!new) {
				fprintf(stderr, PLUGIN_NAME ": Cannot allocate memory for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
				fastfile_close(ff);
				return NULL;
			}
			ff = new;
			ff->size += FASTFILE_INCREMENT_BUFFER;
			x = FASTFILE_INCREMENT_BUFFER;
		}

		r = read(ff->fd, &ff->data[s], ff->size - s);
		if(r == -1) {
			fprintf(stderr, PLUGIN_NAME ": Cannot read from file '%s'. Reason: %s\n", ff->filename, strerror(errno));
			fastfile_close(ff);
			return NULL;
		}

		ff->len += r;
		s = ff->len;
	}

	if(lseek(ff->fd, 0, SEEK_SET) == -1) {
		fprintf(stderr, PLUGIN_NAME ": Cannot rewind on file '%s'.\n", ff->filename);
		fastfile_close(ff);
		return NULL;
	}

	fflines_reset(ff->lines);
	ffwords_reset(ff->words);

	ff = fastfile_parser(ff);

	return ff;
}

fastfile *fastfile_open(const char *filename, const char *separators) {
	if(debug) fprintf(stderr, PLUGIN_NAME ": Opening file '%s'\n", filename);

	int fd = open(filename, O_RDONLY|O_NOATIME, 0666);
	if(fd == -1) {
		fprintf(stderr, PLUGIN_NAME ": Cannot open file '%s'. Reason: %s\n", filename, strerror(errno));
		return NULL;
	}

	fastfile *ff = malloc(sizeof(fastfile) + FASTFILE_INITIAL_BUFFER);
	if(!ff) {
		fprintf(stderr, PLUGIN_NAME ": Cannot allocate memory for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
		close(fd);
		return NULL;
	}

	strncpy(ff->filename, filename, FILENAME_MAX);
	ff->filename[FILENAME_MAX] = '\0';

	ff->fd = fd;
	ff->size = FASTFILE_INITIAL_BUFFER;
	ff->len = 0;

	ff->lines = fflines_new();
	ff->words = ffwords_new();

	if(!ff->lines || !ff->words) {
		fprintf(stderr, PLUGIN_NAME ": Cannot initialize parser for file '%s'. Reason: %s\n", ff->filename, strerror(errno));
		fastfile_close(ff);
		return NULL;
	}

	int i;
	for(i = 0; i < 256 ;i++) {
		if(i == '\n' || i == '\r') ff->separators[i] = CHAR_IS_NEWLINE;
		else if(isspace(i) || !isprint(i)) ff->separators[i] = CHAR_IS_SEPARATOR;
		else ff->separators[i] = CHAR_IS_WORD;
	}

	if(!separators) separators = " \t=|";
	const char *s = separators;
	while(*s) ff->separators[(int)*s++] = CHAR_IS_SEPARATOR;

	return ff;
}

// ----------------------------------------------------------------------------
/*
#define NETDATA_COMMAND_GETOPTION
#define NETDATA_COMMAND_CREATECHART
#define NETDATA_COMMAND_VALUES

typedef struct {
	char magic[5];
	uint16_t version;
	uint32_t transaction;
	uint32_t options;
	uint8_t command;
	uint32_t content_length;
} netdata_msg_header;

typedef struct {
	netdata_msg_header header;
	char section[NETDATA_CONFIG_SECTION_LEN + 1];
	char option[NETDATA_CONFIG_OPTION_LEN + 1];
	uint8_t type;
	union default {
		uint8_t boolean;

	}
} netdata_getoption;

typedef struct {
	netdata_header header;

}
*/

// ----------------------------------------------------------------------------

// return the number of lines present
#define fastfile_lines(ff) (ff->lines->len)

// return the Nth word of the file, or NULL
#define fastfile_word(ff, word) (((word) < (ff)->words->len) ? (ff)->words->words[(word)] : NULL)

// return the first word of the Nth line, or NULL
#define fastfile_line(ff, line) (((line) < fastfile_lines(ff)) ? fastfile_word((ff), (ff)->lines->lines[(line)].first) : NULL)

// return the number of words of the Nth line
#define fastfile_linewords(ff, line) (((line) < fastfile_lines(ff)) ? (ff)->lines->lines[(line)].words : 0)

// return the Nth word of the current line
#define fastfile_lineword(ff, line, word) (((line) < fastfile_lines(ff) && (word) < fastfile_linewords(ff, (line))) ? fastfile_word((ff), (ff)->lines->lines[(line)].first + word) : NULL)

// a basic processor to print the parsed file
void print_processor(fastfile *ff, unsigned long long usec) {
	uint32_t lines = fastfile_lines(ff), l;
	uint32_t words, w;
	char *s;

	fprintf(stderr, PLUGIN_NAME ": File '%s' with %d lines and %d words\n", ff->filename, ff->lines->len, ff->words->len);

	for(l = 0; l < lines ;l++) {
		words = fastfile_linewords(ff, l);

		fprintf(stderr, PLUGIN_NAME ":	line %d starts at word %d and has %d words\n", l, ff->lines->lines[l].first, ff->lines->lines[l].words);

		for(w = 0; w < words ;w++) {
			s = fastfile_lineword(ff, l, w);
			fprintf(stderr, PLUGIN_NAME ":		[%d.%d] '%s'\n", l, w, s);
		}
	}
}


void proc_net_dev_processor(fastfile *ff, unsigned long long usec) {
	uint32_t lines = fastfile_lines(ff), l;
	uint32_t words;

	for(l = 2; l < lines ;l++) {
		words = fastfile_linewords(ff, l);
		if(words < 17) continue;

		fprintf(stdout, "BEGIN net.%s %llu\n",		fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 1));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 9));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_packets.%s %llu\n",	fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 2));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 10));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_errors.%s %llu\n",	fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 3));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 11));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_drops.%s %llu\n",	fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 4));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 12));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_fifo.%s %llu\n",	fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 5));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 13));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_compressed.%s %llu\n", fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET received = %s\n",		fastfile_lineword(ff, l, 7));
		fprintf(stdout, "SET sent = %s\n",		fastfile_lineword(ff, l, 16));
		fprintf(stdout, "END\n");

		fprintf(stdout, "BEGIN net_other.%s %llu\n",	fastfile_lineword(ff, l, 0), usec);
		fprintf(stdout, "SET frames = %s\n",		fastfile_lineword(ff, l, 6));
		fprintf(stdout, "SET multicast = %s\n",		fastfile_lineword(ff, l, 8));
		fprintf(stdout, "SET collisions = %s\n",	fastfile_lineword(ff, l, 14));
		fprintf(stdout, "SET carrier = %s\n",		fastfile_lineword(ff, l, 15));
		fprintf(stdout, "END\n");
	}
}


// ----------------------------------------------------------------------------



typedef struct {
	const char filename[FILENAME_MAX + 1];
	const char separators[256];
	void (*processor)(fastfile *, unsigned long long);
	fastfile *ff;
	int enabled;
} proc_files;

proc_files files[] = {
	{ "/proc/net/dev",			" \t,:|", proc_net_dev_processor, NULL, 1 },
	{ "/proc/diskstats",			" \t,:|", print_processor, NULL, 1 },
	{ "/proc/net/snmp",			" \t,:|", print_processor, NULL, 1 },
	{ "/proc/net/netstat",			" \t,:|", print_processor, NULL, 1 },
	{ "/proc/net/stat/nf_conntrack",	" \t,:|", print_processor, NULL, 1 },
	{ "/proc/net/ip_vs_stats",		" \t,:|", print_processor, NULL, 1 },
	{ "/proc/stat",				" \t,:|", print_processor, NULL, 1 },
	{ "/proc/meminfo",			" \t,:|", print_processor, NULL, 1 },
	{ "/proc/vmstat",			" \t,:|", print_processor, NULL, 1 },
	{ "/proc/self/mountstats",		" \t", print_processor, NULL, 1 },
	{ "/sys/class/thermal/thermal_zone0/temp", "", print_processor, NULL, 1 },
	{ "/sys/devices/system/cpu/cpu3/cpufreq/cpuinfo_cur_freq", "", print_processor, NULL, 1 },
	{ "",					"", NULL, NULL, 0 },
};

void do_proc_files() {
	int i;

	unsigned long started_t = time(NULL), current_t;
	unsigned long long usec = 0, susec = 0;
	struct timeval last, now;
	gettimeofday(&last, NULL);

	while(1) {
		gettimeofday(&now, NULL);
		usec = usecdiff(&now, &last) - susec;

		for(i = 0; files[i].filename[0] ;i++) {
			if(!files[i].enabled) continue;

			if(debug) fprintf(stderr, PLUGIN_NAME ": File '%s'\n", files[i].filename);

			if(!files[i].ff) {
				files[i].ff = fastfile_open(files[i].filename, files[i].separators);
				if(!files[i].ff) {
					files[i].enabled = 0;
					continue;
				}
			}

			if(files[i].ff) files[i].ff = fastfile_readall(files[i].ff);
			if(files[i].ff) files[i].processor(files[i].ff, usec + susec);
		}

		fprintf(stderr, PLUGIN_NAME ": Last loop took %llu usec (worked for %llu, sleeped for %llu).\n", usec + susec, usec, susec);
		if(debug) {
			exit(1);
		}

		// if the last loop took less than half the time
		// wait the rest of the time
		if(usec < (update_every * 1000000ULL / 2)) susec = (update_every * 1000000ULL) - usec;
		else susec = update_every * 1000000ULL / 2;

		usleep(susec);
		bcopy(&now, &last, sizeof(struct timeval));

		current_t = time(NULL);
		if(current_t - started_t > 3600) exit(0);
	}
}

// ----------------------------------------------------------------------------
// parse command line arguments

void parse_args(int argc, char **argv)
{
	int i, freq = 0;

	for(i = 1; i < argc; i++) {
		if(!freq) {
			int n = atoi(argv[i]);
			if(n > 0) {
				freq = n;
				continue;
			}
		}

		if(strcmp("debug", argv[i]) == 0) {
			debug = 1;
			continue;
		}

		fprintf(stderr, PLUGIN_NAME ": ERROR: cannot understand option %s\n", argv[i]);
		exit(1);
	}

	if(freq > 0) update_every = freq;
}

int main(int argc, char **argv)
{
	parse_args(argc, argv);

	// show_charts();
	do_proc_files();
	return 0;
}
