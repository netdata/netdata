/* SPDX-License-Identifier: GPL-3.0+ */
#include <stdio.h>
#include <inttypes.h>
#include <string.h>
#include <stdlib.h>
#include <ctype.h>
#include <sys/time.h>

#define likely(x) __builtin_expect(!!(x), 1)
#define unlikely(x) __builtin_expect(!!(x), 0)

#define simple_hash(name) ({                                         \
    register unsigned char *__hash_source = (unsigned char *)(name); \
    register uint32_t __hash_value = 0x811c9dc5;                     \
    while (*__hash_source) {                                         \
        __hash_value *= 16777619;                                    \
        __hash_value ^= (uint32_t) *__hash_source++;                 \
    }                                                                \
    __hash_value;                                                    \
})

static inline uint32_t simple_hash2(const char *name) {
    register unsigned char *s = (unsigned char *)name;
    register uint32_t hval = 0x811c9dc5;
    while (*s) {
        hval *= 16777619;
        hval ^= (uint32_t) *s++;
    }
    return hval;
}

static inline unsigned long long fast_strtoull(const char *s) {
  register unsigned long long n = 0;
  register char c;
  for(c = *s; c >= '0' && c <= '9' ; c = *(++s)) {
    n *= 10;
    n += c - '0';
    // n = (n << 1) + (n << 3) + (c - '0');
  }
  return n;
}

static uint32_t cache_hash = 0;
static uint32_t rss_hash = 0;
static uint32_t rss_huge_hash = 0;
static uint32_t mapped_file_hash = 0;
static uint32_t writeback_hash = 0;
static uint32_t dirty_hash = 0;
static uint32_t swap_hash = 0;
static uint32_t pgpgin_hash = 0;
static uint32_t pgpgout_hash = 0;
static uint32_t pgfault_hash = 0;
static uint32_t pgmajfault_hash = 0;
static uint32_t inactive_anon_hash = 0;
static uint32_t active_anon_hash = 0;
static uint32_t inactive_file_hash = 0;
static uint32_t active_file_hash = 0;
static uint32_t unevictable_hash = 0;
static uint32_t hierarchical_memory_limit_hash = 0;
static uint32_t total_cache_hash = 0;
static uint32_t total_rss_hash = 0;
static uint32_t total_rss_huge_hash = 0;
static uint32_t total_mapped_file_hash = 0;
static uint32_t total_writeback_hash = 0;
static uint32_t total_dirty_hash = 0;
static uint32_t total_swap_hash = 0;
static uint32_t total_pgpgin_hash = 0;
static uint32_t total_pgpgout_hash = 0;
static uint32_t total_pgfault_hash = 0;
static uint32_t total_pgmajfault_hash = 0;
static uint32_t total_inactive_anon_hash = 0;
static uint32_t total_active_anon_hash = 0;
static uint32_t total_inactive_file_hash = 0;
static uint32_t total_active_file_hash = 0;
static uint32_t total_unevictable_hash = 0;

char *strings[] = {
  "cache",
  "rss",
  "rss_huge",
  "mapped_file",
  "writeback",
  "dirty",
  "swap",
  "pgpgin",
  "pgpgout",
  "pgfault",
  "pgmajfault",
  "inactive_anon",
  "active_anon",
  "inactive_file",
  "active_file",
  "unevictable",
  "hierarchical_memory_limit",
  "total_cache",
  "total_rss",
  "total_rss_huge",
  "total_mapped_file",
  "total_writeback",
  "total_dirty",
  "total_swap",
  "total_pgpgin",
  "total_pgpgout",
  "total_pgfault",
  "total_pgmajfault",
  "total_inactive_anon",
  "total_active_anon",
  "total_inactive_file",
  "total_active_file",
  "total_unevictable",
  NULL
};

unsigned long long values1[12] = { 0 };
unsigned long long values2[12] = { 0 };
unsigned long long values3[12] = { 0 };
unsigned long long values4[12] = { 0 };
unsigned long long values5[12] = { 0 };
unsigned long long values6[12] = { 0 };

#define NUMBER1  "12345678901234"
#define NUMBER2  "23456789012345"
#define NUMBER3  "34567890123456"
#define NUMBER4  "45678901234567"
#define NUMBER5  "56789012345678"
#define NUMBER6  "67890123456789"
#define NUMBER7  "78901234567890"
#define NUMBER8  "89012345678901"
#define NUMBER9  "90123456789012"
#define NUMBER10 "12345678901234"
#define NUMBER11 "23456789012345"

// simple system strcmp()
void test1() {
  int i;
  for(i = 0; strings[i] ; i++) {
    char *s = strings[i];

    if(unlikely(!strcmp(s, "cache")))
      values1[i] = strtoull(NUMBER1, NULL, 10);

    else if(unlikely(!strcmp(s, "rss")))
      values1[i] = strtoull(NUMBER2, NULL, 10);
    
    else if(unlikely(!strcmp(s, "rss_huge")))
      values1[i] = strtoull(NUMBER3, NULL, 10);
  
    else if(unlikely(!strcmp(s, "mapped_file")))
      values1[i] = strtoull(NUMBER4, NULL, 10);
    
    else if(unlikely(!strcmp(s, "writeback")))
      values1[i] = strtoull(NUMBER5, NULL, 10);
    
    else if(unlikely(!strcmp(s, "dirty")))
      values1[i] = strtoull(NUMBER6, NULL, 10);
    
    else if(unlikely(!strcmp(s, "swap")))
      values1[i] = strtoull(NUMBER7, NULL, 10);
    
    else if(unlikely(!strcmp(s, "pgpgin")))
      values1[i] = strtoull(NUMBER8, NULL, 10);
    
    else if(unlikely(!strcmp(s, "pgpgout")))
      values1[i] = strtoull(NUMBER9, NULL, 10);
    
    else if(unlikely(!strcmp(s, "pgfault")))
      values1[i] = strtoull(NUMBER10, NULL, 10);
    
    else if(unlikely(!strcmp(s, "pgmajfault")))
      values1[i] = strtoull(NUMBER11, NULL, 10);
  }
}

// inline simple_hash() with system strtoull()
void test2() {
  int i;
  for(i = 0; strings[i] ; i++) {
    char *s = strings[i];
    uint32_t hash = simple_hash2(s);

    if(unlikely(hash == cache_hash && !strcmp(s, "cache")))
      values2[i] = strtoull(NUMBER1, NULL, 10);
    
    else if(unlikely(hash == rss_hash && !strcmp(s, "rss")))
      values2[i] = strtoull(NUMBER2, NULL, 10);
    
    else if(unlikely(hash == rss_huge_hash && !strcmp(s, "rss_huge")))
      values2[i] = strtoull(NUMBER3, NULL, 10);
    
    else if(unlikely(hash == mapped_file_hash && !strcmp(s, "mapped_file")))
      values2[i] = strtoull(NUMBER4, NULL, 10);
    
    else if(unlikely(hash == writeback_hash && !strcmp(s, "writeback")))
      values2[i] = strtoull(NUMBER5, NULL, 10);
    
    else if(unlikely(hash == dirty_hash && !strcmp(s, "dirty")))
      values2[i] = strtoull(NUMBER6, NULL, 10);
    
    else if(unlikely(hash == swap_hash && !strcmp(s, "swap")))
      values2[i] = strtoull(NUMBER7, NULL, 10);
  
    else if(unlikely(hash == pgpgin_hash && !strcmp(s, "pgpgin")))
      values2[i] = strtoull(NUMBER8, NULL, 10);
    
    else if(unlikely(hash == pgpgout_hash && !strcmp(s, "pgpgout")))
      values2[i] = strtoull(NUMBER9, NULL, 10);
    
    else if(unlikely(hash == pgfault_hash && !strcmp(s, "pgfault")))
      values2[i] = strtoull(NUMBER10, NULL, 10);
    
    else if(unlikely(hash == pgmajfault_hash && !strcmp(s, "pgmajfault")))
      values2[i] = strtoull(NUMBER11, NULL, 10);
  }
}

// statement expression simple_hash(), system strtoull()
void test3() {
  int i;
  for(i = 0; strings[i] ; i++) {
    char *s = strings[i];
    uint32_t hash = simple_hash(s);

    if(unlikely(hash == cache_hash && !strcmp(s, "cache")))
      values3[i] = strtoull(NUMBER1, NULL, 10);
    
    else if(unlikely(hash == rss_hash && !strcmp(s, "rss")))
      values3[i] = strtoull(NUMBER2, NULL, 10);
    
    else if(unlikely(hash == rss_huge_hash && !strcmp(s, "rss_huge")))
      values3[i] = strtoull(NUMBER3, NULL, 10);
    
    else if(unlikely(hash == mapped_file_hash && !strcmp(s, "mapped_file")))
      values3[i] = strtoull(NUMBER4, NULL, 10);
    
    else if(unlikely(hash == writeback_hash && !strcmp(s, "writeback")))
      values3[i] = strtoull(NUMBER5, NULL, 10);
    
    else if(unlikely(hash == dirty_hash && !strcmp(s, "dirty")))
      values3[i] = strtoull(NUMBER6, NULL, 10);
    
    else if(unlikely(hash == swap_hash && !strcmp(s, "swap")))
      values3[i] = strtoull(NUMBER7, NULL, 10);
    
    else if(unlikely(hash == pgpgin_hash && !strcmp(s, "pgpgin")))
      values3[i] = strtoull(NUMBER8, NULL, 10);
    
    else if(unlikely(hash == pgpgout_hash && !strcmp(s, "pgpgout")))
      values3[i] = strtoull(NUMBER9, NULL, 10);
    
    else if(unlikely(hash == pgfault_hash && !strcmp(s, "pgfault")))
      values3[i] = strtoull(NUMBER10, NULL, 10);
    
    else if(unlikely(hash == pgmajfault_hash && !strcmp(s, "pgmajfault")))
      values3[i] = strtoull(NUMBER11, NULL, 10);
  }
}


// inline simple_hash(), if-continue checks
void test4() {
  int i;
  for(i = 0; strings[i] ; i++) {
    char *s = strings[i];
    uint32_t hash = simple_hash2(s);

    if(unlikely(hash == cache_hash && !strcmp(s, "cache"))) {
      values4[i] = strtoull(NUMBER1, NULL, 0);
      continue;
    }

    if(unlikely(hash == rss_hash && !strcmp(s, "rss"))) {
      values4[i] = strtoull(NUMBER2, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == rss_huge_hash && !strcmp(s, "rss_huge"))) {
      values4[i] = strtoull(NUMBER3, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == mapped_file_hash && !strcmp(s, "mapped_file"))) {
      values4[i] = strtoull(NUMBER4, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == writeback_hash && !strcmp(s, "writeback"))) {
      values4[i] = strtoull(NUMBER5, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == dirty_hash && !strcmp(s, "dirty"))) {
      values4[i] = strtoull(NUMBER6, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == swap_hash && !strcmp(s, "swap"))) {
      values4[i] = strtoull(NUMBER7, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == pgpgin_hash && !strcmp(s, "pgpgin"))) {
      values4[i] = strtoull(NUMBER8, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == pgpgout_hash && !strcmp(s, "pgpgout"))) {
      values4[i] = strtoull(NUMBER9, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == pgfault_hash && !strcmp(s, "pgfault"))) {
      values4[i] = strtoull(NUMBER10, NULL, 0);
      continue;
    }
    
    if(unlikely(hash == pgmajfault_hash && !strcmp(s, "pgmajfault"))) {
      values4[i] = strtoull(NUMBER11, NULL, 0);
      continue;
    }
  }
}

// inline simple_hash(), if-else-if-else-if (netdata default)
void test5() {
  int i;
  for(i = 0; strings[i] ; i++) {
    char *s = strings[i];
    uint32_t hash = simple_hash2(s);

    if(unlikely(hash == cache_hash && !strcmp(s, "cache")))
      values5[i] = fast_strtoull(NUMBER1);
    
    else if(unlikely(hash == rss_hash && !strcmp(s, "rss")))
      values5[i] = fast_strtoull(NUMBER2);
    
    else if(unlikely(hash == rss_huge_hash && !strcmp(s, "rss_huge")))
      values5[i] = fast_strtoull(NUMBER3);
    
    else if(unlikely(hash == mapped_file_hash && !strcmp(s, "mapped_file")))
      values5[i] = fast_strtoull(NUMBER4);
    
    else if(unlikely(hash == writeback_hash && !strcmp(s, "writeback")))
      values5[i] = fast_strtoull(NUMBER5);
    
    else if(unlikely(hash == dirty_hash && !strcmp(s, "dirty")))
      values5[i] = fast_strtoull(NUMBER6);
    
    else if(unlikely(hash == swap_hash && !strcmp(s, "swap")))
      values5[i] = fast_strtoull(NUMBER7);
  
    else if(unlikely(hash == pgpgin_hash && !strcmp(s, "pgpgin")))
      values5[i] = fast_strtoull(NUMBER8);
    
    else if(unlikely(hash == pgpgout_hash && !strcmp(s, "pgpgout")))
      values5[i] = fast_strtoull(NUMBER9);
    
    else if(unlikely(hash == pgfault_hash && !strcmp(s, "pgfault")))
      values5[i] = fast_strtoull(NUMBER10);
    
    else if(unlikely(hash == pgmajfault_hash && !strcmp(s, "pgmajfault")))
      values5[i] = fast_strtoull(NUMBER11);
  }
}

// ----------------------------------------------------------------------------

struct entry {
  char *name;
  uint32_t hash;
  int found;
  void (*func)(void *data1, void *data2);
  void *data1;
  void *data2;
  struct entry *prev, *next;
};

struct base {
  int iteration;
  int registered;
  int wanted;
  int found;
  struct entry *entries, *last;
};

static inline void callback(void *data1, void *data2) {
  char *string = data1;
  unsigned long long *value = data2;
  *value = fast_strtoull(string);
}


static inline struct base *entry(struct base *base, const char *name, void *data1, void *data2, void (*func)(void *, void *)) {
  if(!base)
    base = calloc(1, sizeof(struct base));

  struct entry *e = malloc(sizeof(struct entry));
  e->name = strdup(name);
  e->hash = simple_hash2(e->name);
  e->data1 = data1;
  e->data2 = data2;
  e->func = func;
  e->prev = NULL;
  e->next = base->entries;

  if(base->entries) base->entries->prev = e;
  else base->last = e;

  base->entries = e;
  base->registered++;
  base->wanted = base->registered;

  return base;
}

static inline int check(struct base *base, const char *s) {
  uint32_t hash = simple_hash2(s);

  if(likely(hash == base->last->hash && !strcmp(s, base->last->name))) {
    base->last->found = 1;
    base->found++;
    if(base->last->func) base->last->func(base->last->data1, base->last->data2);
    base->last = base->last->next;

    if(!base->last)
      base->last = base->entries;

    if(base->found == base->registered)
      return 1;

    return 0;
  }

  // find it
  struct entry *e;
  for(e = base->entries; e ; e = e->next)
    if(e->hash == hash && !strcmp(e->name, s))
      break;

  if(e == base->last) {
    printf("ERROR\n");
    exit(1);
  }

  if(e) {
    // found

    // run it
    if(e->func) e->func(e->data1, e->data2);

    // unlink it
    if(e->next) e->next->prev = e->prev;
    if(e->prev) e->prev->next = e->next;

    if(base->entries == e)
      base->entries = e->next;
  }
  else {
    // not found

    // create it
    e = calloc(1, sizeof(struct entry));
    e->name = strdup(s);
    e->hash = hash;
  }

  // link it here
  e->next = base->last;
  if(base->last) {
    e->prev = base->last->prev;
    base->last->prev = e;

    if(base->entries == base->last)
      base->entries = e;
  }
  else
    e->prev = NULL;

  if(e->prev)
    e->prev->next = e;

  base->last = e->next;
  if(!base->last)
    base->last = base->entries;

  e->found = 1;
  base->found++;

  if(base->found == base->registered)
    return 1;

  printf("relinked '%s' after '%s' and before '%s': ", e->name, e->prev?e->prev->name:"NONE", e->next?e->next->name:"NONE");
  for(e = base->entries; e ; e = e->next) printf("%s ", e->name);
  printf("\n");

  return 0;
}

static inline void begin(struct base *base) {

  if(unlikely(base->iteration % 60) == 1) {
    base->wanted = 0;
    struct entry *e;
    for(e = base->entries; e ; e = e->next)
      if(e->found) base->wanted++;
  }

  base->iteration++;
  base->last = base->entries;
  base->found = 0;
}

void test6() {

  static struct base *base = NULL;

  if(unlikely(!base)) {
    base = entry(base, "cache", NUMBER1, &values6[0], callback);
    base = entry(base, "rss", NUMBER2, &values6[1], callback);
    base = entry(base, "rss_huge", NUMBER3, &values6[2], callback);
    base = entry(base, "mapped_file", NUMBER4, &values6[3], callback);
    base = entry(base, "writeback", NUMBER5, &values6[4], callback);
    base = entry(base, "dirty", NUMBER6, &values6[5], callback);
    base = entry(base, "swap", NUMBER7, &values6[6], callback);
    base = entry(base, "pgpgin", NUMBER8, &values6[7], callback);
    base = entry(base, "pgpgout", NUMBER9, &values6[8], callback);
    base = entry(base, "pgfault", NUMBER10, &values6[9], callback);
    base = entry(base, "pgmajfault", NUMBER11, &values6[10], callback);
  }

  begin(base);

  int i;
  for(i = 0; strings[i] ; i++) {
    if(check(base, strings[i]))
      break;
  }
}

// ----------------------------------------------------------------------------


// ==============
// --- Poor man cycle counting.
static unsigned long tsc;

static void begin_tsc(void)
{
  unsigned long a, d;
  asm volatile ("cpuid\nrdtsc" : "=a" (a), "=d" (d) : "0" (0) : "ebx", "ecx");
  tsc = ((unsigned long)d << 32) | (unsigned long)a;
}

static unsigned long end_tsc(void)
{
  unsigned long a, d;
  asm volatile ("rdtscp" : "=a" (a), "=d" (d) : : "ecx");
  return (((unsigned long)d << 32) | (unsigned long)a) - tsc;
}
// ===============

static unsigned long long clk;

static void begin_clock() {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1))
        return;
    clk = tv.tv_sec  * 1000000 + tv.tv_usec;
}

static unsigned long long end_clock() {
    struct timeval tv;
    if(unlikely(gettimeofday(&tv, NULL) == -1))
        return -1;
    return clk = tv.tv_sec  * 1000000 + tv.tv_usec - clk;
}

void main(void)
{
    cache_hash = simple_hash("cache");
    rss_hash = simple_hash("rss");
    rss_huge_hash = simple_hash("rss_huge");
    mapped_file_hash = simple_hash("mapped_file");
    writeback_hash = simple_hash("writeback");
    dirty_hash = simple_hash("dirty");
    swap_hash = simple_hash("swap");
    pgpgin_hash = simple_hash("pgpgin");
    pgpgout_hash = simple_hash("pgpgout");
    pgfault_hash = simple_hash("pgfault");
    pgmajfault_hash = simple_hash("pgmajfault");
    inactive_anon_hash = simple_hash("inactive_anon");
    active_anon_hash = simple_hash("active_anon");
    inactive_file_hash = simple_hash("inactive_file");
    active_file_hash = simple_hash("active_file");
    unevictable_hash = simple_hash("unevictable");
    hierarchical_memory_limit_hash = simple_hash("hierarchical_memory_limit");
    total_cache_hash = simple_hash("total_cache");
    total_rss_hash = simple_hash("total_rss");
    total_rss_huge_hash = simple_hash("total_rss_huge");
    total_mapped_file_hash = simple_hash("total_mapped_file");
    total_writeback_hash = simple_hash("total_writeback");
    total_dirty_hash = simple_hash("total_dirty");
    total_swap_hash = simple_hash("total_swap");
    total_pgpgin_hash = simple_hash("total_pgpgin");
    total_pgpgout_hash = simple_hash("total_pgpgout");
    total_pgfault_hash = simple_hash("total_pgfault");
    total_pgmajfault_hash = simple_hash("total_pgmajfault");
    total_inactive_anon_hash = simple_hash("total_inactive_anon");
    total_active_anon_hash = simple_hash("total_active_anon");
    total_inactive_file_hash = simple_hash("total_inactive_file");
    total_active_file_hash = simple_hash("total_active_file");
    total_unevictable_hash = simple_hash("total_unevictable");

    // cache functions
    (void)simple_hash2("hello world");
    (void)strcmp("1", "2");
    (void)strtoull("123", NULL, 0);

  unsigned long i, c1 = 0, c2 = 0, c3 = 0, c4 = 0, c5 = 0, c6 = 0;
  unsigned long max = 200000;

  begin_clock();
  for(i = 0; i <= max ;i++) test1();
  c1 = end_clock();

  begin_clock();
  for(i = 0; i <= max ;i++) test2();
  c2 = end_clock();
    
  begin_clock();
  for(i = 0; i <= max ;i++) test3();
  c3 = end_clock();

  begin_clock();
  for(i = 0; i <= max ;i++) test4();
  c4 = end_clock();

  begin_clock();
  for(i = 0; i <= max ;i++) test5();
  c5 = end_clock();

  begin_clock();
  for(i = 0; i <= max ;i++) test6();
  c6 = end_clock();

  for(i = 0; i < 11 ; i++)
    printf("value %lu: %llu %llu %llu %llu %llu %llu\n", i, values1[i], values2[i], values3[i], values4[i], values5[i], values6[i]);
  
  printf("\n\nRESULTS\n");
  printf("test1() in %lu usecs: simple system strcmp().\n"
         "test2() in %lu usecs: inline simple_hash() with system strtoull().\n"
         "test3() in %lu usecs: statement expression simple_hash(), system strtoull().\n"
         "test4() in %lu usecs: inline simple_hash(), if-continue checks.\n"
         "test5() in %lu usecs: inline simple_hash(), if-else-if-else-if (netdata default).\n"
         "test6() in %lu usecs: adaptive re-sortable array (wow!)\n"
         , c1
         , c2
         , c3
         , c4
         , c5
         , c6
         );

}
