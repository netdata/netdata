#ifdef HAVE_CONFIG_H
#include <config.h>
#endif
#include <sys/syscall.h>
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
#include "appconfig.h"
#include "../config.h"

char *global_host_prefix = "";
int enable_ksm = 1;

// time(NULL) in milliseconds
unsigned long long timems(void) {
	struct timeval now;
	gettimeofday(&now, NULL);
	return now.tv_sec * 1000000ULL + now.tv_usec;
}

unsigned char netdata_map_chart_names[256] = {
		[0] = '\0', //
		[1] = '_', //
		[2] = '_', //
		[3] = '_', //
		[4] = '_', //
		[5] = '_', //
		[6] = '_', //
		[7] = '_', //
		[8] = '_', //
		[9] = '_', //
		[10] = '_', //
		[11] = '_', //
		[12] = '_', //
		[13] = '_', //
		[14] = '_', //
		[15] = '_', //
		[16] = '_', //
		[17] = '_', //
		[18] = '_', //
		[19] = '_', //
		[20] = '_', //
		[21] = '_', //
		[22] = '_', //
		[23] = '_', //
		[24] = '_', //
		[25] = '_', //
		[26] = '_', //
		[27] = '_', //
		[28] = '_', //
		[29] = '_', //
		[30] = '_', //
		[31] = '_', //
		[32] = '_', //
		[33] = '_', // !
		[34] = '_', // "
		[35] = '_', // #
		[36] = '_', // $
		[37] = '_', // %
		[38] = '_', // &
		[39] = '_', // '
		[40] = '_', // (
		[41] = '_', // )
		[42] = '_', // *
		[43] = '_', // +
		[44] = '.', // ,
		[45] = '-', // -
		[46] = '.', // .
		[47] = '/', // /
		[48] = '0', // 0
		[49] = '1', // 1
		[50] = '2', // 2
		[51] = '3', // 3
		[52] = '4', // 4
		[53] = '5', // 5
		[54] = '6', // 6
		[55] = '7', // 7
		[56] = '8', // 8
		[57] = '9', // 9
		[58] = '_', // :
		[59] = '_', // ;
		[60] = '_', // <
		[61] = '_', // =
		[62] = '_', // >
		[63] = '_', // ?
		[64] = '_', // @
		[65] = 'a', // A
		[66] = 'b', // B
		[67] = 'c', // C
		[68] = 'd', // D
		[69] = 'e', // E
		[70] = 'f', // F
		[71] = 'g', // G
		[72] = 'h', // H
		[73] = 'i', // I
		[74] = 'j', // J
		[75] = 'k', // K
		[76] = 'l', // L
		[77] = 'm', // M
		[78] = 'n', // N
		[79] = 'o', // O
		[80] = 'p', // P
		[81] = 'q', // Q
		[82] = 'r', // R
		[83] = 's', // S
		[84] = 't', // T
		[85] = 'u', // U
		[86] = 'v', // V
		[87] = 'w', // W
		[88] = 'x', // X
		[89] = 'y', // Y
		[90] = 'z', // Z
		[91] = '_', // [
		[92] = '/', // backslash
		[93] = '_', // ]
		[94] = '_', // ^
		[95] = '_', // _
		[96] = '_', // `
		[97] = 'a', // a
		[98] = 'b', // b
		[99] = 'c', // c
		[100] = 'd', // d
		[101] = 'e', // e
		[102] = 'f', // f
		[103] = 'g', // g
		[104] = 'h', // h
		[105] = 'i', // i
		[106] = 'j', // j
		[107] = 'k', // k
		[108] = 'l', // l
		[109] = 'm', // m
		[110] = 'n', // n
		[111] = 'o', // o
		[112] = 'p', // p
		[113] = 'q', // q
		[114] = 'r', // r
		[115] = 's', // s
		[116] = 't', // t
		[117] = 'u', // u
		[118] = 'v', // v
		[119] = 'w', // w
		[120] = 'x', // x
		[121] = 'y', // y
		[122] = 'z', // z
		[123] = '_', // {
		[124] = '_', // |
		[125] = '_', // }
		[126] = '_', // ~
		[127] = '_', //
		[128] = '_', //
		[129] = '_', //
		[130] = '_', //
		[131] = '_', //
		[132] = '_', //
		[133] = '_', //
		[134] = '_', //
		[135] = '_', //
		[136] = '_', //
		[137] = '_', //
		[138] = '_', //
		[139] = '_', //
		[140] = '_', //
		[141] = '_', //
		[142] = '_', //
		[143] = '_', //
		[144] = '_', //
		[145] = '_', //
		[146] = '_', //
		[147] = '_', //
		[148] = '_', //
		[149] = '_', //
		[150] = '_', //
		[151] = '_', //
		[152] = '_', //
		[153] = '_', //
		[154] = '_', //
		[155] = '_', //
		[156] = '_', //
		[157] = '_', //
		[158] = '_', //
		[159] = '_', //
		[160] = '_', //
		[161] = '_', //
		[162] = '_', //
		[163] = '_', //
		[164] = '_', //
		[165] = '_', //
		[166] = '_', //
		[167] = '_', //
		[168] = '_', //
		[169] = '_', //
		[170] = '_', //
		[171] = '_', //
		[172] = '_', //
		[173] = '_', //
		[174] = '_', //
		[175] = '_', //
		[176] = '_', //
		[177] = '_', //
		[178] = '_', //
		[179] = '_', //
		[180] = '_', //
		[181] = '_', //
		[182] = '_', //
		[183] = '_', //
		[184] = '_', //
		[185] = '_', //
		[186] = '_', //
		[187] = '_', //
		[188] = '_', //
		[189] = '_', //
		[190] = '_', //
		[191] = '_', //
		[192] = '_', //
		[193] = '_', //
		[194] = '_', //
		[195] = '_', //
		[196] = '_', //
		[197] = '_', //
		[198] = '_', //
		[199] = '_', //
		[200] = '_', //
		[201] = '_', //
		[202] = '_', //
		[203] = '_', //
		[204] = '_', //
		[205] = '_', //
		[206] = '_', //
		[207] = '_', //
		[208] = '_', //
		[209] = '_', //
		[210] = '_', //
		[211] = '_', //
		[212] = '_', //
		[213] = '_', //
		[214] = '_', //
		[215] = '_', //
		[216] = '_', //
		[217] = '_', //
		[218] = '_', //
		[219] = '_', //
		[220] = '_', //
		[221] = '_', //
		[222] = '_', //
		[223] = '_', //
		[224] = '_', //
		[225] = '_', //
		[226] = '_', //
		[227] = '_', //
		[228] = '_', //
		[229] = '_', //
		[230] = '_', //
		[231] = '_', //
		[232] = '_', //
		[233] = '_', //
		[234] = '_', //
		[235] = '_', //
		[236] = '_', //
		[237] = '_', //
		[238] = '_', //
		[239] = '_', //
		[240] = '_', //
		[241] = '_', //
		[242] = '_', //
		[243] = '_', //
		[244] = '_', //
		[245] = '_', //
		[246] = '_', //
		[247] = '_', //
		[248] = '_', //
		[249] = '_', //
		[250] = '_', //
		[251] = '_', //
		[252] = '_', //
		[253] = '_', //
		[254] = '_', //
		[255] = '_'  //
};

// make sure the supplied string
// is good for a netdata chart/dimension ID/NAME
void netdata_fix_chart_name(char *s) {
	while((*s = netdata_map_chart_names[(unsigned char)*s])) s++;
}

unsigned char netdata_map_chart_ids[256] = {
		[0] = '\0', //
		[1] = '_', //
		[2] = '_', //
		[3] = '_', //
		[4] = '_', //
		[5] = '_', //
		[6] = '_', //
		[7] = '_', //
		[8] = '_', //
		[9] = '_', //
		[10] = '_', //
		[11] = '_', //
		[12] = '_', //
		[13] = '_', //
		[14] = '_', //
		[15] = '_', //
		[16] = '_', //
		[17] = '_', //
		[18] = '_', //
		[19] = '_', //
		[20] = '_', //
		[21] = '_', //
		[22] = '_', //
		[23] = '_', //
		[24] = '_', //
		[25] = '_', //
		[26] = '_', //
		[27] = '_', //
		[28] = '_', //
		[29] = '_', //
		[30] = '_', //
		[31] = '_', //
		[32] = '_', //
		[33] = '_', // !
		[34] = '_', // "
		[35] = '_', // #
		[36] = '_', // $
		[37] = '_', // %
		[38] = '_', // &
		[39] = '_', // '
		[40] = '_', // (
		[41] = '_', // )
		[42] = '_', // *
		[43] = '_', // +
		[44] = '.', // ,
		[45] = '-', // -
		[46] = '.', // .
		[47] = '_', // /
		[48] = '0', // 0
		[49] = '1', // 1
		[50] = '2', // 2
		[51] = '3', // 3
		[52] = '4', // 4
		[53] = '5', // 5
		[54] = '6', // 6
		[55] = '7', // 7
		[56] = '8', // 8
		[57] = '9', // 9
		[58] = '_', // :
		[59] = '_', // ;
		[60] = '_', // <
		[61] = '_', // =
		[62] = '_', // >
		[63] = '_', // ?
		[64] = '_', // @
		[65] = 'a', // A
		[66] = 'b', // B
		[67] = 'c', // C
		[68] = 'd', // D
		[69] = 'e', // E
		[70] = 'f', // F
		[71] = 'g', // G
		[72] = 'h', // H
		[73] = 'i', // I
		[74] = 'j', // J
		[75] = 'k', // K
		[76] = 'l', // L
		[77] = 'm', // M
		[78] = 'n', // N
		[79] = 'o', // O
		[80] = 'p', // P
		[81] = 'q', // Q
		[82] = 'r', // R
		[83] = 's', // S
		[84] = 't', // T
		[85] = 'u', // U
		[86] = 'v', // V
		[87] = 'w', // W
		[88] = 'x', // X
		[89] = 'y', // Y
		[90] = 'z', // Z
		[91] = '_', // [
		[92] = '/', // backslash
		[93] = '_', // ]
		[94] = '_', // ^
		[95] = '_', // _
		[96] = '_', // `
		[97] = 'a', // a
		[98] = 'b', // b
		[99] = 'c', // c
		[100] = 'd', // d
		[101] = 'e', // e
		[102] = 'f', // f
		[103] = 'g', // g
		[104] = 'h', // h
		[105] = 'i', // i
		[106] = 'j', // j
		[107] = 'k', // k
		[108] = 'l', // l
		[109] = 'm', // m
		[110] = 'n', // n
		[111] = 'o', // o
		[112] = 'p', // p
		[113] = 'q', // q
		[114] = 'r', // r
		[115] = 's', // s
		[116] = 't', // t
		[117] = 'u', // u
		[118] = 'v', // v
		[119] = 'w', // w
		[120] = 'x', // x
		[121] = 'y', // y
		[122] = 'z', // z
		[123] = '_', // {
		[124] = '_', // |
		[125] = '_', // }
		[126] = '_', // ~
		[127] = '_', //
		[128] = '_', //
		[129] = '_', //
		[130] = '_', //
		[131] = '_', //
		[132] = '_', //
		[133] = '_', //
		[134] = '_', //
		[135] = '_', //
		[136] = '_', //
		[137] = '_', //
		[138] = '_', //
		[139] = '_', //
		[140] = '_', //
		[141] = '_', //
		[142] = '_', //
		[143] = '_', //
		[144] = '_', //
		[145] = '_', //
		[146] = '_', //
		[147] = '_', //
		[148] = '_', //
		[149] = '_', //
		[150] = '_', //
		[151] = '_', //
		[152] = '_', //
		[153] = '_', //
		[154] = '_', //
		[155] = '_', //
		[156] = '_', //
		[157] = '_', //
		[158] = '_', //
		[159] = '_', //
		[160] = '_', //
		[161] = '_', //
		[162] = '_', //
		[163] = '_', //
		[164] = '_', //
		[165] = '_', //
		[166] = '_', //
		[167] = '_', //
		[168] = '_', //
		[169] = '_', //
		[170] = '_', //
		[171] = '_', //
		[172] = '_', //
		[173] = '_', //
		[174] = '_', //
		[175] = '_', //
		[176] = '_', //
		[177] = '_', //
		[178] = '_', //
		[179] = '_', //
		[180] = '_', //
		[181] = '_', //
		[182] = '_', //
		[183] = '_', //
		[184] = '_', //
		[185] = '_', //
		[186] = '_', //
		[187] = '_', //
		[188] = '_', //
		[189] = '_', //
		[190] = '_', //
		[191] = '_', //
		[192] = '_', //
		[193] = '_', //
		[194] = '_', //
		[195] = '_', //
		[196] = '_', //
		[197] = '_', //
		[198] = '_', //
		[199] = '_', //
		[200] = '_', //
		[201] = '_', //
		[202] = '_', //
		[203] = '_', //
		[204] = '_', //
		[205] = '_', //
		[206] = '_', //
		[207] = '_', //
		[208] = '_', //
		[209] = '_', //
		[210] = '_', //
		[211] = '_', //
		[212] = '_', //
		[213] = '_', //
		[214] = '_', //
		[215] = '_', //
		[216] = '_', //
		[217] = '_', //
		[218] = '_', //
		[219] = '_', //
		[220] = '_', //
		[221] = '_', //
		[222] = '_', //
		[223] = '_', //
		[224] = '_', //
		[225] = '_', //
		[226] = '_', //
		[227] = '_', //
		[228] = '_', //
		[229] = '_', //
		[230] = '_', //
		[231] = '_', //
		[232] = '_', //
		[233] = '_', //
		[234] = '_', //
		[235] = '_', //
		[236] = '_', //
		[237] = '_', //
		[238] = '_', //
		[239] = '_', //
		[240] = '_', //
		[241] = '_', //
		[242] = '_', //
		[243] = '_', //
		[244] = '_', //
		[245] = '_', //
		[246] = '_', //
		[247] = '_', //
		[248] = '_', //
		[249] = '_', //
		[250] = '_', //
		[251] = '_', //
		[252] = '_', //
		[253] = '_', //
		[254] = '_', //
		[255] = '_'  //
};

// make sure the supplied string
// is good for a netdata chart/dimension ID/NAME
void netdata_fix_chart_id(char *s) {
	while((*s = netdata_map_chart_ids[(unsigned char)*s])) s++;
}

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
		// NOTE: No need to optimize with left shifts.
		//       GCC will use imul instruction anyway.
		//       Tested with 'gcc -O3 -S'
		//hval += (hval<<1) + (hval<<4) + (hval<<7) + (hval<<8) + (hval<<24);
		hval *= 16777619;

		// xor the bottom with the current octet
		hval ^= (uint32_t)*s++;
	}

	// fprintf(stderr, "HASH: %u = %s\n", hval, name);
	return hval;
}

uint32_t simple_uhash(const char *name) {
	unsigned char *s = (unsigned char *)name;
	uint32_t hval = 0x811c9dc5, c;

	// FNV-1a algorithm
	while((c = *s++)) {
		if(unlikely(c >= 'A' && c <= 'Z')) c += 'a' - 'A';
		hval *= 16777619;
		hval ^= c;
	}
	return hval;
}

/*
// http://eternallyconfuzzled.com/tuts/algorithms/jsw_tut_hashing.aspx
// one at a time hash
uint32_t simple_hash(const char *name) {
	unsigned char *s = (unsigned char *)name;
	uint32_t h = 0;

	while(*s) {
		h += *s++;
		h += (h << 10);
		h ^= (h >> 6);
	}

	h += (h << 3);
	h ^= (h >> 11);
	h += (h << 15);

	// fprintf(stderr, "HASH: %u = %s\n", h, name);

	return h;
}
*/

void strreverse(char* begin, char* end)
{
	char aux;

	while (end > begin)
	{
		// clearer code.
		aux = *end;
		*end-- = *begin;
		*begin++ = aux;
	}
}

char *mystrsep(char **ptr, char *s)
{
	char *p = "";
	while ( p && !p[0] && *ptr ) p = strsep(ptr, s);
	return(p);
}

char *trim(char *s)
{
	// skip leading spaces
	// and 'comments' as well!?
	while(*s && isspace(*s)) s++;
	if(!*s || *s == '#') return NULL;

	// skip tailing spaces
	// this way is way faster. Writes only one NUL char.
	ssize_t l = strlen(s);
  if (--l >= 0)
	{
		char *p = s + l;
		while (p > s && isspace(*p)) p--;
		*++p = '\0';
	}

	if(!*s) return NULL;

	return s;
}

void *mymmap(const char *filename, size_t size, int flags, int ksm)
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

#ifdef MADV_MERGEABLE
				if(flags & MAP_SHARED || !enable_ksm || !ksm) {
#endif
					mem = mmap(NULL, size, PROT_READ|PROT_WRITE, flags, fd, 0);
					if(mem) {
						int advise = MADV_SEQUENTIAL|MADV_DONTFORK;
						if(flags & MAP_SHARED) advise |= MADV_WILLNEED;

						if(madvise(mem, size, advise) != 0)
							error("Cannot advise the kernel about the memory usage of file '%s'.", filename);
					}
#ifdef MADV_MERGEABLE
				}
				else {
					mem = mmap(NULL, size, PROT_READ|PROT_WRITE, flags|MAP_ANONYMOUS, -1, 0);
					if(mem) {
						if(lseek(fd, 0, SEEK_SET) == 0) {
							if(read(fd, mem, size) != (ssize_t)size)
								error("Cannot read from file '%s'", filename);
						}
						else
							error("Cannot seek to beginning of file '%s'.", filename);

						// don't use MADV_SEQUENTIAL|MADV_DONTFORK, they disable MADV_MERGEABLE
						if(madvise(mem, size, MADV_SEQUENTIAL|MADV_DONTFORK) != 0)
							error("Cannot advise the kernel about the memory usage (MADV_SEQUENTIAL|MADV_DONTFORK) of file '%s'.", filename);

						if(madvise(mem, size, MADV_MERGEABLE) != 0)
							error("Cannot advise the kernel about the memory usage (MADV_MERGEABLE) of file '%s'.", filename);
					}
					else
						error("Cannot allocate PRIVATE ANONYMOUS memory for KSM for file '%s'.", filename);
				}
#endif
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

	snprintfz(tmpfilename, FILENAME_MAX, "%s.%ld.tmp", filename, (long)getpid());

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

/*
 ***************************************************************************
 * Get number of clock ticks per second.
 ***************************************************************************
 */
unsigned int hz;

void get_HZ(void)
{
	long ticks;

	if ((ticks = sysconf(_SC_CLK_TCK)) == -1) {
		perror("sysconf");
	}

	hz = (unsigned int) ticks;
}

pid_t gettid(void)
{
	return syscall(SYS_gettid);
}

char *fgets_trim_len(char *buf, size_t buf_size, FILE *fp, size_t *len) {
	char *s = fgets(buf, buf_size, fp);
	if(!s) return NULL;

	char *t = s;
	if(*t != '\0') {
		// find the string end
		while (*++t != '\0');

		// trim trailing spaces/newlines/tabs
		while (--t > s && *t == '\n')
			*t = '\0';
	}

	if(len)
		*len = t - s + 1;

	return s;
}

char *strncpyz(char *dst, const char *src, size_t n) {
	char *p = dst;

	while(*src && n--)
		*dst++ = *src++;

	*dst = '\0';

	return p;
}

int vsnprintfz(char *dst, size_t n, const char *fmt, va_list args) {
	int size;

	size = vsnprintf(dst, n, fmt, args);

	if(unlikely((size_t)size > n)) {
		// there is bug in vsnprintf() and it returns
		// a number higher to len, but it does not
		// overflow the buffer.
		size = n;
	}

	dst[size] = '\0';
	return size;
}

int snprintfz(char *dst, size_t n, const char *fmt, ...) {
	va_list args;

	va_start(args, fmt);
	return vsnprintfz(dst, n, fmt, args);
	va_end(args);
}
