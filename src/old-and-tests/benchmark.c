#define _GNU_SOURCE

#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <errno.h>
#include <fcntl.h>
#include <string.h>
#include <malloc.h>
#include <inttypes.h>

#include <sys/types.h>
#include <sys/stat.h>
#include <sys/mman.h>

#define FASTFILE_NORMAL 1

#ifdef FASTFILE_NORMAL
#define FASTFILE_BUFFER 65536
#define FASTFILE_MAX_LINE_LENGTH 4096
#endif

typedef struct {
	int fd;

	ssize_t cursor;

	char buffer[FASTFILE_BUFFER + 1];
	ssize_t size;
	int eof;
} fastfile;

fastfile *fastfile_open(const char *filename) {
	fastfile *ff = malloc(sizeof(fastfile));
	if(!ff) goto cleanup;

	ff->fd = open(filename, O_RDONLY|O_NOATIME, 0666);
	if(ff->fd == -1) goto cleanup;

	ff->cursor = ff->size = ff->eof = 0;
	ff->buffer[0] = '\0';

	ssize_t r = read(ff->fd, &ff->buffer[ff->size], FASTFILE_BUFFER);
	if(r != -1) {
		ff->buffer[r + 1] = '\0';
		ff->size = r;
		if(r < FASTFILE_BUFFER) ff->eof = 1;
	}
	else goto cleanup;

	return ff;

cleanup:
	if(ff) {
		if(ff->fd != -1) close(ff->fd);
		free(ff);
	}
	return NULL;
}

void fastfile_close(fastfile *ff) {
	if(ff->fd != -1) close(ff->fd);
	free(ff);
}

char *fastfile_getline(fastfile *ff) {
	if( !ff->eof && ff->size == FASTFILE_BUFFER && ( ff->cursor + FASTFILE_MAX_LINE_LENGTH ) > ff->size ) {
		// if(ff->size) fprintf(stderr, "Read more\n");

		memmove(ff->buffer, &ff->buffer[ff->cursor], ff->size - ff->cursor + 1);
		ff->size -= ff->cursor;
		ff->cursor = 0;

		ssize_t remaining = FASTFILE_BUFFER - ff->size;
		// fprintf(stderr, "Reading up to %ld bytes\n", remaining);
		ssize_t r = read(ff->fd, &ff->buffer[ff->size], remaining);
		if(r != -1) {
			ff->buffer[r + 1] = '\0';
			ff->size = r;
			// fprintf(stderr, "Read %ld bytes\n", r);
		}

		if(!r || r < remaining) {
			// fprintf(stderr, "Read EOF\n");
			ff->eof = 1;
		}
	}

	//fprintf(stderr, "Line starts at %ld\n", ff->cursor);
	if(ff->cursor >= ff->size) return NULL;

	char *start = &ff->buffer[ff->cursor];
	char *s = start;
	
	while(*s != '\n' && *s != '\0') s++;
	*s = '\0';
	ff->cursor += ( s - start + 1 );
	//fprintf(stderr, "Line ends at %ld\n", ff->cursor);

	return start;
}

const char *filenames[] = {
	"/proc/net/dev",
	"/proc/diskstats",
	"/proc/net/snmp",
	"/proc/net/netstat",
//	"/proc/net/stat/nf_conntrack",
//	"/proc/net/ip_vs_stats",
	"/proc/stat",
	"/proc/meminfo",
	"/proc/vmstat",
	"/proc/self/mountstats",
//	"/var/log/messages",
	NULL
};

int main(int argc, char **argv)
{
	int i, k = 0;
	fastfile *ff;
	char *s;

	for(i = 0; i < 400000 ;i++, k++) {
		if(filenames[k] == NULL) k = 0;

		//fprintf(stderr, "\nOpening file '%s'\n", filenames[k]);
		ff = fastfile_open(filenames[k]);
		if(!ff) {
			fprintf(stderr, "Cannot open file '%s', reason: %s\n", filenames[k], strerror(errno));
			exit(1);
		}

		while((s = fastfile_getline(ff))) ;
		//	fprintf(stderr, "%s\n", s);

		fastfile_close(ff);
	}
	return 0;
}
