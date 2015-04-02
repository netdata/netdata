// enable O_NOATIME
#define _GNU_SOURCE

#include <string.h>
#include <ctype.h>
#include <errno.h>
#include <unistd.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>
#include <sys/mman.h>


#include "log.h"
#include "common.h"

/*
// http://stackoverflow.com/questions/7666509/hash-function-for-string
uint32_t simple_hash(const char *name)
{
	const char *s = name;
	uint32_t hash = 5381;
	int i;

	while((i = *s++)) hash = ((hash << 5) + hash) + i;

	// fprintf(stderr, "HASH: %lu %s\n", hash, name);

	return hash;
}
*/

// http://isthe.com/chongo/tech/comp/fnv/#FNV-1a
uint32_t simple_hash(const char *name) {
	unsigned char *s = (unsigned char *)name;
	uint32_t hval = 0x811c9dc5;

	// FNV-1a algorithm
	while (*s) {
		// multiply by the 32 bit FNV magic prime mod 2^32
		// gcc optimized
		hval += (hval<<1) + (hval<<4) + (hval<<7) + (hval<<8) + (hval<<24);

		// xor the bottom with the current octet
		hval ^= (uint32_t)*s++;
	}

	// fprintf(stderr, "HASH: %u = %s\n", hval, name);
	return hval;
}


void strreverse(char* begin, char* end)
{
    char aux;
    while (end > begin)
        aux = *end, *end-- = *begin, *begin++ = aux;
}

char *mystrsep(char **ptr, char *s)
{
	char *p = "";
	while ( p && !p[0] && *ptr ) p = strsep(ptr, s);
	return(p);
}

// like strsep() but:
// it trims spaces before and after each value
// it accepts quoted values in single or double quotes
char *qstrsep(char **ptr)
{
	if(!*ptr || !**ptr) return NULL;
	
	char *s, *p = *ptr;

	// skip leading spaces
	while(isspace(*p)) p++;

	// if the first char is a quote, assume quoted
	if(*p == '"' || *p == '\'') {
		char q = *p;
		s = ++p;
		while(*p && *p != q) p++;

		if(*p == q) {
			*p = '\0';
			p++;
		}

		*ptr = p;
		return s;
	}

	s = p;
	while(*p && !isspace(*p)) p++;
	if(!*p) *ptr = NULL;
	else {
		*p = '\0';
		*ptr = ++p;
	}

	return s;
}

char *trim(char *s)
{
	// skip leading spaces
	while(*s && isspace(*s)) s++;
	if(!*s || *s == '#') return NULL;

	// skip tailing spaces
	int c = strlen(s) - 1;
	while(c >= 0 && isspace(s[c])) {
		s[c] = '\0';
		c--;
	}
	if(c < 0) return NULL;
	if(!*s) return NULL;
	return s;
}

void *mymmap(const char *filename, unsigned long size, int flags)
{
	int fd;
	void *mem = NULL;

	errno = 0;
	fd = open(filename, O_RDWR|O_CREAT|O_NOATIME, 0664);
	if(fd != -1) {
		if(lseek(fd, size, SEEK_SET) == (long)size) {
			if(write(fd, "", 1) == 1) {

				if(ftruncate(fd, size))
					error("Cannot truncate file '%s' to size %ld. Will use the larger file.", filename, size);

				mem = mmap(NULL, size, PROT_READ|PROT_WRITE, flags, fd, 0);
				if(mem) {
					if(madvise(mem, size, MADV_SEQUENTIAL|MADV_DONTFORK|MADV_WILLNEED) != 0)
						error("Cannot advise the kernel about the memory usage of file '%s'.", filename);
				}
			}
			else error("Cannot write to file '%s' at position %ld.", filename, size);
		}
		else error("Cannot seek file '%s' to size %ld.", filename, size);

		close(fd);
	}
	else error("Cannot create/open file '%s'.", filename);

	return mem;
}

int savememory(const char *filename, void *mem, unsigned long size)
{
	char tmpfilename[FILENAME_MAX + 1];

	snprintf(tmpfilename, FILENAME_MAX, "%s.%ld.tmp", filename, (long)getpid());

	int fd = open(tmpfilename, O_RDWR|O_CREAT|O_NOATIME, 0664);
	if(fd < 0) {
		error("Cannot create/open file '%s'.", filename);
		return -1;
	}

	if(write(fd, mem, size) != (long)size) {
		error("Cannot write to file '%s' %ld bytes.", filename, (long)size);
		close(fd);
		return -1;
	}

	close(fd);

	int ret = 0;
	if(rename(tmpfilename, filename)) {
		error("Cannot rename '%s' to '%s'", tmpfilename, filename);
		ret = -1;
	}

	return ret;
}

int fd_is_valid(int fd) {
    return fcntl(fd, F_GETFD) != -1 || errno != EBADF;
}

