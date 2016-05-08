#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

// gcc -O1 -ggdb -Wall -Wextra -I ../src/ -I ../ -o registry ../src/registry.c ../src/dictionary.o ../src/log.o ../src/avl.o ../src/common.o ../src/appconfig.o ../src/web_buffer.o ../src/storage_number.o  -pthread -luuid -lm -DHAVE_CONFIG_H -DVARLIB_DIR="\"/tmp\""

#include <uuid/uuid.h>
#include <inttypes.h>
#include <stdlib.h>
#include <string.h>
#include <ctype.h>
#include <unistd.h>
#include <sys/stat.h>
#include <sys/types.h>
#include <errno.h>

#include "log.h"
#include "common.h"
#include "dictionary.h"
#include "appconfig.h"

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

#define DICTIONARY_FLAGS DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE | DICTIONARY_FLAG_NAME_LINK_DONT_CLONE

// ----------------------------------------------------------------------------
// COMMON structures

struct registry {
	unsigned long long persons_count;
	unsigned long long machines_count;
	unsigned long long usages_count;
	unsigned long long urls_count;
	unsigned long long persons_urls_count;
	unsigned long long machines_urls_count;

	char *pathname;
	char *db_filename;
	char *log_filename;
	FILE *registry_log_fp;

	DICTIONARY *persons; 	// dictionary of PERSON *, with key the PERSON.guid
	DICTIONARY *machines; 	// dictionary of MACHINE *, with key the MACHINE.guid
	DICTIONARY *urls; 		// dictionary of URL *, with key the URL.url

} registry;


// ----------------------------------------------------------------------------
// URL structures
// Save memory by de-duplicating URLs

struct url {
	uint32_t links;
	uint16_t len;
	char url[1];
};
typedef struct url URL;


// ----------------------------------------------------------------------------
// MACHINE structures

// For each MACHINE-URL pair we keep this
struct machine_url {
	URL *url;					// de-duplicated URL
//	DICTIONARY *persons;		// dictionary of PERSON *

	uint8_t flags;
	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
};
typedef struct machine_url MACHINE_URL;

// A machine
struct machine {
	char guid[36 + 1];

	DICTIONARY *urls; 			// MACHINE_URL *

	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
};
typedef struct machine MACHINE;


// ----------------------------------------------------------------------------
// PERSON structures

// for each PERSON-URL pair we keep this
struct person_url {
	URL *url;				// de-duplicated URL
	MACHINE *machine;

	uint8_t flags;
	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
};
typedef struct person_url PERSON_URL;

// A person
struct person {
	char guid[36 + 1];

	DICTIONARY *urls; // PERSON_URL *

	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
};
typedef struct person PERSON;

extern PERSON *registry_request(const char *person_guid, const char *machine_guid, const char *url, time_t when);

// ----------------------------------------------------------------------------
// URL

static inline URL *registry_url_allocate(const char *url) {
	size_t len = strlen(url);

	debug(D_REGISTRY, "Registry: registry_url_allocate('%s'): allocating %zu bytes", url, sizeof(URL) + len);
	URL *u = malloc(sizeof(URL) + len);
	if(!u) fatal("Cannot allocate %zu bytes for URL '%s'", sizeof(URL) + len);

	strcpy(u->url, url);
	u->len = len;
	u->links = 0;

	debug(D_REGISTRY, "Registry: registry_url_allocate('%s'): indexing it", url);
	dictionary_set(registry.urls, u->url, u, sizeof(URL));

	return u;
}

static inline URL *registry_url_get(const char *url) {
	debug(D_REGISTRY, "Registry: registry_url_get('%s')", url);

	URL *u = dictionary_get(registry.urls, url);
	if(!u) {
		u = registry_url_allocate(url);
		registry.urls_count++;
	}

	return u;
}

static inline void registry_url_link(URL *u) {
	u->links++;
	debug(D_REGISTRY, "Registry: registry_url_unlink('%s'): This URL has now %u links", u->url, u->links);
}

static inline void registry_url_unlink(URL *u) {
	u->links--;
	if(!u->links) {
		debug(D_REGISTRY, "Registry: registry_url_unlink('%s'): No more links for this URL", u->url);
		dictionary_del(registry.urls, u->url);
		free(u);
	}
	else
		debug(D_REGISTRY, "Registry: registry_url_unlink('%s'): This URL has %u links left", u->url, u->links);
}


// ----------------------------------------------------------------------------
// MACHINE

static inline MACHINE *registry_machine_find(const char *machine_guid) {
	debug(D_REGISTRY, "Registry: registry_machine_find('%s')", machine_guid);
	return dictionary_get(registry.machines, machine_guid);
}

static inline MACHINE_URL *registry_machine_url_allocate(MACHINE *m, URL *u, time_t when) {
	debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): allocating %zu bytes", m->guid, u->url, sizeof(MACHINE_URL));

	MACHINE_URL *mu = malloc(sizeof(MACHINE_URL));
	if(!mu) fatal("registry_machine_link_to_url('%s', '%s'): cannot allocate %zu bytes.", m->guid, u->url, sizeof(MACHINE_URL));

	// mu->persons = dictionary_create(DICTIONARY_FLAGS);
	// dictionary_set(mu->persons, p->guid, p, sizeof(PERSON));

	mu->first_t = mu->last_t = when;
	mu->usages = 1;
	mu->url = u;
	mu->flags = REGISTRY_URL_FLAGS_DEFAULT;

	debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s'): indexing URL in machine", m->guid, u->url);
	dictionary_set(m->urls, u->url, mu, sizeof(MACHINE_URL));
	registry_url_link(u);

	return mu;
}

static inline MACHINE *registry_machine_allocate(const char *machine_guid, time_t when) {
	debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating new machine, sizeof(MACHINE)=%zu", machine_guid, sizeof(MACHINE));

	MACHINE *m = calloc(1, sizeof(MACHINE));
	if(!m) fatal("Registry: cannot allocate memory for new machine '%s'", machine_guid);

	strncpy(m->guid, machine_guid, 36);

	debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating dictionary of urls", machine_guid);
	m->urls = dictionary_create(DICTIONARY_FLAGS);

	m->first_t = m->last_t = when;
	m->usages = 0;

	dictionary_set(registry.machines, m->guid, m, sizeof(MACHINE));

	return m;
}

// 1. validate machine GUID
// 2. if it is valid, find it or create it and return it
// 3. if it is not valid, return NULL
static inline MACHINE *registry_machine_get(const char *machine_guid, time_t when) {
	MACHINE *m = NULL;

	if(likely(machine_guid && *machine_guid)) {
		// validate it is a GUID
		uuid_t uuid;
		if(uuid_parse(machine_guid, uuid) == -1) {
			info("Registry: machine guid '%s' is not a valid guid. Ignoring it.", machine_guid);
		}
		else {
			char buf[36 + 1];
			uuid_unparse_lower(uuid, buf);
			if(strcmp(machine_guid, buf))
				info("Registry: machine guid '%s' and re-generated '%s' differ!", machine_guid, buf);

			machine_guid = buf;
			m = registry_machine_find(machine_guid);
			if(!m) {
				m = registry_machine_allocate(machine_guid, when);
				registry.machines_count++;
			}
		}
	}

	return m;
}


// ----------------------------------------------------------------------------
// PERSON

static inline PERSON *registry_person_find(const char *person_guid) {
	debug(D_REGISTRY, "Registry: registry_person_find('%s')", person_guid);
	return dictionary_get(registry.persons, person_guid);
}

static inline PERSON_URL *registry_person_url_allocate(PERSON *p, MACHINE *m, URL *u, time_t when) {
	debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): allocating %zu bytes", p->guid, m->guid, u->url, sizeof(PERSON_URL));
	PERSON_URL *pu = malloc(sizeof(PERSON_URL));
	if(!pu) fatal("registry_person_link_to_url('%s', '%s', '%s'): cannot allocate %zu bytes.", p->guid, m->guid, u->url, sizeof(PERSON_URL));

	pu->machine = m;
	pu->first_t = pu->last_t = when;
	pu->usages = 1;
	pu->url = u;
	pu->flags = REGISTRY_URL_FLAGS_DEFAULT;

	debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): indexing URL in person", p->guid, m->guid, u->url);
	dictionary_set(p->urls, u->url, pu, sizeof(PERSON_URL));
	registry_url_link(u);

	return pu;
}

static inline PERSON *registry_person_allocate(const char *person_guid, time_t when) {
	PERSON *p = NULL;

	debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): allocating new person, sizeof(PERSON)=%zu", (person_guid)?person_guid:"", sizeof(PERSON));

	p = calloc(1, sizeof(PERSON));
	if(!p) fatal("Registry: cannot allocate memory for new person.");

	if(!person_guid) {
		for (; ;) {
			uuid_t uuid;
			if (uuid_generate_time_safe(uuid) == -1)
				info("Registry: uuid_generate_time_safe() reports UUID generation is not safe for uniqueness.");

			uuid_unparse_lower(uuid, p->guid);

			debug(D_REGISTRY, "Registry: Checking if the generated person guid '%s' is unique", p->guid);
			if (!dictionary_get(registry.persons, p->guid)) {
				debug(D_REGISTRY, "Registry: generated person guid '%s' is unique", p->guid);
				break;
			}
			else
				info("Registry: generated person guid '%s' found in the registry. Retrying...", p->guid);
		}
	}
	else {
		strncpy(p->guid, person_guid, 36);
		p->guid[36] = '\0';
	}

	debug(D_REGISTRY, "Registry: registry_person_allocate('%s'): creating dictionary of urls", p->guid);
	p->urls = dictionary_create(DICTIONARY_FLAGS);

	p->first_t = p->last_t = when;
	p->usages = 0;

	dictionary_set(registry.persons, p->guid, p, sizeof(PERSON));
	return p;
}


// 1. validate person GUID
// 2. if it is valid, find it
// 3. if it is not valid, create a new one
// 4. return it
static inline PERSON *registry_person_get(const char *person_guid, time_t when) {
	PERSON *p = NULL;

	if(person_guid && *person_guid) {
		// validate it is a GUID
		uuid_t uuid;
		if(uuid_parse(person_guid, uuid) == -1) {
			info("Registry: person guid '%s' is not a valid guid. Ignoring it.", person_guid);
		}
		else {
			char buf[36 + 1];
			uuid_unparse_lower(uuid, buf);
			if(strcmp(person_guid, buf))
				info("Registry: person guid '%s' and re-generated '%s' differ!", person_guid, buf);

			person_guid = buf;
			p = registry_person_find(person_guid);
			if(!p) person_guid = NULL;
		}
	}

	if(!p) {
		p = registry_person_allocate(NULL, when);
		registry.persons_count++;
	}

	return p;
}

// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

static inline PERSON_URL *registry_person_link_to_url(PERSON *p, MACHINE *m, URL *u, time_t when) {
	debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): searching for URL in person", p->guid, m->guid, u->url);

	PERSON_URL *pu = dictionary_get(p->urls, u->url);
	if(!pu) {
		debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, u->url);
		pu = registry_person_url_allocate(p, m, u, when);
		registry.persons_urls_count++;
	}
	else {
		debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
		pu->usages++;

		if(pu->machine != m) {
			MACHINE_URL *mu = dictionary_get(pu->machine->urls, u->url);
			if(mu) {
				info("registry_person_link_to_url('%s', '%s', '%s'): URL switched machines (old was '%s') - expiring it from previous machine.",
					 p->guid, m->guid, u->url, pu->machine->guid);
				mu->flags |= REGISTRY_URL_FLAGS_EXPIRED;
			}

			pu->machine = m;
		}
	}

	p->usages++;
	p->last_t = when;

	if(pu->flags & REGISTRY_URL_FLAGS_EXPIRED)
		info("registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL.", p->guid, m->guid, u->url);

	return pu;
}

static inline MACHINE_URL *registry_machine_link_to_url(PERSON *p, MACHINE *m, URL *u, time_t when) {
	debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): searching for URL in machine", p->guid, m->guid, u->url);

	MACHINE_URL *mu = dictionary_get(m->urls, u->url);
	if(!mu) {
		debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): not found", p->guid, m->guid, u->url);
		mu = registry_machine_url_allocate(m, u, when);
		registry.machines_urls_count++;
	}
	else {
		debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
		mu->usages++;
	}

	//debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): indexing person in machine", p->guid, m->guid, u->url);
	//dictionary_set(mu->persons, p->guid, p, sizeof(PERSON));

	m->usages++;
	m->last_t = when;

	if(mu->flags & REGISTRY_URL_FLAGS_EXPIRED)
		info("registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL.", p->guid, m->guid, u->url);

	return mu;
}

// ----------------------------------------------------------------------------
// REGISTRY LOG LOAD/SAVE

static inline void registry_log(const char action, PERSON *p, MACHINE *m, URL *u) {
	if(likely(registry.registry_log_fp))
		fprintf(registry.registry_log_fp, "%c\t%08x\t%s\t%s\t%s\n",
				action,
				p->last_t,
				p->guid,
				m->guid,
				u->url
		);
}

void registry_log_open(void) {
	registry.registry_log_fp = fopen(registry.log_filename, "a");

	if(registry.registry_log_fp) {
		if (setvbuf(registry.registry_log_fp, NULL, _IOLBF, 0) != 0)
			error("Cannot set line buffering on registry log file.");
	}
}

void registry_log_close(void) {
	if(registry.registry_log_fp)
		fclose(registry.registry_log_fp);

	registry.registry_log_fp = NULL;
}

void registry_log_recreate(void) {
	if(registry.registry_log_fp != NULL) {
		fclose(registry.registry_log_fp);

		registry.registry_log_fp = fopen(registry.log_filename, "w");
		if(registry.registry_log_fp) fclose(registry.registry_log_fp);
		registry.registry_log_fp = NULL;

		registry_log_open();
	}
}

int registry_log_load(void) {
	char *s, buf[4096 + 1];
	size_t line = 0;

	registry_log_close();

	debug(D_REGISTRY, "Registry: loading active db from: %s", registry.log_filename);
	FILE *fp = fopen(registry.log_filename, "r");
	if(!fp) {
		error("Registry: cannot open registry file: %s", registry.log_filename);
		return 0;
	}

	size_t len = 0;
	while((s = fgets_trim_len(buf, 4096, fp, &len))) {
		line++;

		switch(s[0]) {
			case 'A':
				// verify it is valid
				if(unlikely(len < 85 || s[1] != '\t' || s[10] != '\t' || s[47] != '\t' || s[84] != '\t'))
					error("Registry log line %u is wrong (len = %zu).", line, len);

				s[1] = s[10] = s[47] = s[84] = '\0';
				registry_request(&s[11], &s[48], &s[85], strtoul(&s[2], NULL, 16));
				break;

			default:
				error("Registry: ignoring line %zu of filename '%s': %s.", line, registry.log_filename, s);
				break;
		}
	}

	registry_log_open();

	return 0;
}


// ----------------------------------------------------------------------------
// REGISTRY REQUESTS

PERSON *registry_request(const char *person_guid, const char *machine_guid, const char *url, time_t when) {
	debug(D_REGISTRY, "registry_request('%s', '%s', '%s'): NEW REQUEST", (person_guid)?person_guid:"", machine_guid, url);

	MACHINE *m = registry_machine_get(machine_guid, when);
	if(!m) return NULL;

	URL *u = registry_url_get(url);
	PERSON *p = registry_person_get(person_guid, when);

	registry_person_link_to_url(p, m, u, when);
	registry_machine_link_to_url(p, m, u, when);

	registry_log('A', p, m, u);

	registry.usages_count++;
	return p;
}


// ----------------------------------------------------------------------------
// REGISTRY LOAD/SAVE

int registry_machine_save_url(void *entry, void *file) {
	MACHINE_URL *mu = entry;
	FILE *fp = file;

	debug(D_REGISTRY, "Registry: registry_machine_save_url('%s')", mu->url->url);

	int ret = fprintf(fp, "V\t%08x\t%08x\t%08x\t%02x\t%s\n",
			mu->first_t,
			mu->last_t,
			mu->usages,
			mu->flags,
			mu->url->url
	);

	return ret;
}

int registry_machine_save(void *entry, void *file) {
	MACHINE *m = entry;
	FILE *fp = file;

	debug(D_REGISTRY, "Registry: registry_machine_save('%s')", m->guid);

	int ret = fprintf(fp, "M\t%08x\t%08x\t%08x\t%s\n",
			m->first_t,
			m->last_t,
			m->usages,
			m->guid
	);

	if(ret >= 0) {
		int ret2 = dictionary_get_all(m->urls, registry_machine_save_url, fp);
		if(ret2 < 0) return ret2;
		ret += ret2;
	}

	return ret;
}

static inline int registry_person_save_url(void *entry, void *file) {
	PERSON_URL *pu = entry;
	FILE *fp = file;

	debug(D_REGISTRY, "Registry: registry_person_save_url('%s')", pu->url->url);

	int ret = fprintf(fp, "U\t%08x\t%08x\t%08x\t%02x\t%s\t%s\n",
			pu->first_t,
			pu->last_t,
			pu->usages,
			pu->flags,
			pu->machine->guid,
			pu->url->url
	);

	return ret;
}

static inline int registry_person_save(void *entry, void *file) {
	PERSON *p = entry;
	FILE *fp = file;

	debug(D_REGISTRY, "Registry: registry_person_save('%s')", p->guid);

	int ret = fprintf(fp, "P\t%08x\t%08x\t%08x\t%s\n",
			p->first_t,
			p->last_t,
			p->usages,
			p->guid
	);

	if(ret >= 0) {
		int ret2 = dictionary_get_all(p->urls, registry_person_save_url, fp);
		if (ret2 < 0) return ret2;
		ret += ret2;
	}

	return ret;
}

static int registry_save(void) {
	char tmp_filename[FILENAME_MAX + 1];
	char old_filename[FILENAME_MAX + 1];

	snprintf(old_filename, FILENAME_MAX, "%s.old", registry.db_filename);
	snprintf(tmp_filename, FILENAME_MAX, "%s.tmp", registry.db_filename);

	debug(D_REGISTRY, "Registry: Creating file '%s'", tmp_filename);
	FILE *fp = fopen(tmp_filename, "w");
	if(!fp) {
		error("Registry: Cannot create file: %s", tmp_filename);
		return -1;
	}

	debug(D_REGISTRY, "Saving all machines");
	int bytes1 = dictionary_get_all(registry.machines, registry_machine_save, fp);
	if(bytes1 < 0) {
		error("Registry: Cannot save registry machines - return value %d", bytes1);
		fclose(fp);
		return bytes1;
	}
	debug(D_REGISTRY, "Registry: saving machines took %d bytes", bytes1);

	debug(D_REGISTRY, "Saving all persons");
	int bytes2 = dictionary_get_all(registry.persons, registry_person_save, fp);
	if(bytes2 < 0) {
		error("Registry: Cannot save registry persons - return value %d", bytes2);
		fclose(fp);
		return bytes2;
	}
	debug(D_REGISTRY, "Registry: saving persons took %d bytes", bytes2);

	fclose(fp);

	errno = 0;

	// remove the .old db
	debug(D_REGISTRY, "Registry: Removing old db '%s'", old_filename);
	if(unlink(old_filename) == -1 && errno != ENOENT)
		error("Registry: cannot remove old registry file '%s'", old_filename);

	// rename the db to .old
	debug(D_REGISTRY, "Registry: Link current db '%s' to .old: '%s'", registry.db_filename, old_filename);
	if(link(registry.db_filename, old_filename) == -1 && errno != ENOENT)
		error("Registry: cannot move file '%s' to '%s'. Saving registry DB failed!", tmp_filename, registry.db_filename);

	else {
		// remove the database (it is saved in .old)
		debug(D_REGISTRY, "Registry: removing db '%s'", registry.db_filename);
		if (unlink(registry.db_filename) == -1 && errno != ENOENT)
			error("Registry: cannot remove old registry file '%s'", registry.db_filename);

		// move the .tmp to make it active
		debug(D_REGISTRY, "Registry: linking tmp db '%s' to active db '%s'", tmp_filename, registry.db_filename);
		if (link(tmp_filename, registry.db_filename) == -1) {
			error("Registry: cannot move file '%s' to '%s'. Saving registry DB failed!", tmp_filename,
				  registry.db_filename);

			// move the .old back
			debug(D_REGISTRY, "Registry: linking old db '%s' to active db '%s'", old_filename, registry.db_filename);
			if(link(old_filename, registry.db_filename) == -1)
				error("Registry: cannot move file '%s' to '%s'. Recovering the old registry DB failed!", old_filename, registry.db_filename);
		}
		else {
			debug(D_REGISTRY, "Registry: removing tmp db '%s'", tmp_filename);
			if(unlink(tmp_filename) == -1)
				error("Registry: cannot remove tmp registry file '%s'", tmp_filename);

			// it has been moved successfully
			// discard the current registry log
			registry_log_recreate();
		}
	}

	return -1;
}

static inline size_t registry_load(void) {
	char *s, buf[4096 + 1];
	PERSON *p = NULL;
	MACHINE *m = NULL;
	URL *u = NULL;
	size_t line = 0;

	debug(D_REGISTRY, "Registry: loading active db from: %s", registry.db_filename);
	FILE *fp = fopen(registry.db_filename, "r");
	if(!fp) {
		error("Registry: cannot open registry file: %s", registry.db_filename);
		return 0;
	}

	size_t len = 0;
	while((s = fgets_trim_len(buf, 4096, fp, &len))) {
		line++;

		debug(D_REGISTRY, "Registry: read line %zu to length %zu: %s", line, len, s);

		switch(*s) {
			case 'P': // person
				m = NULL;
				// verify it is valid
				if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0'))
					error("Registry person line %u is wrong (len = %zu).", line, len);

				s[1] = s[10] = s[19] = s[28] = '\0';
				p = registry_person_allocate(&s[29], strtoul(&s[2], NULL, 16));
				p->last_t = strtoul(&s[11], NULL, 16);
				p->usages = strtoul(&s[20], NULL, 16);
				debug(D_REGISTRY, "Registry loaded person '%s', first: %u, last: %u, usages: %u", p->guid, p->first_t, p->last_t, p->usages);
				break;

			case 'M': // machine
				p = NULL;
				// verify it is valid
				if(unlikely(len != 65 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[65] != '\0'))
					error("Registry person line %u is wrong (len = %zu).", line, len);

				s[1] = s[10] = s[19] = s[28] = '\0';
				m = registry_machine_allocate(&s[29], strtoul(&s[2], NULL, 16));
				m->last_t = strtoul(&s[11], NULL, 16);
				m->usages = strtoul(&s[20], NULL, 16);
				debug(D_REGISTRY, "Registry loaded machine '%s', first: %u, last: %u, usages: %u", m->guid, m->first_t, m->last_t, m->usages);
				break;

			case 'U': // person URL
				if(unlikely(!p)) {
					error("Registry: ignoring line %zu, no person loaded: %s", line, s);
					break;
				}

				// verify it is valid
				if(len < 69 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t' || s[68] != '\t')
					error("Registry person URL line %u is wrong (len = %zu).", line, len);

				s[1] = s[10] = s[19] = s[28] = s[31] = s[68] = '\0';
				u = registry_url_allocate(&s[69]);

				time_t first_t = strtoul(&s[2], NULL, 16);

				m = registry_machine_find(&s[32]);
				if(!m) m = registry_machine_allocate(&s[32], first_t);

				PERSON_URL *pu = registry_person_url_allocate(p, m, u, first_t);
				pu->last_t = strtoul(&s[11], NULL, 16);
				pu->usages = strtoul(&s[20], NULL, 16);
				pu->flags = strtoul(&s[29], NULL, 16);
				debug(D_REGISTRY, "Registry loaded person URL '%s', machine '%s', first: %u, last: %u, usages: %u, flags: %02x", u->url, m->guid, pu->first_t, pu->last_t, pu->usages, pu->flags);
				break;

			case 'V': // machine URL
				if(unlikely(!m)) {
					error("Registry: ignoring line %zu, no machine loaded: %s", line, s);
					break;
				}

				// verify it is valid
				if(len < 32 || s[1] != '\t' || s[10] != '\t' || s[19] != '\t' || s[28] != '\t' || s[31] != '\t')
					error("Registry person URL line %u is wrong (len = %zu).", line, len);

				s[1] = s[10] = s[19] = s[28] = s[31] = '\0';
				u = registry_url_allocate(&s[32]);

				MACHINE_URL *mu = registry_machine_url_allocate(m, u, strtoul(&s[2], NULL, 16));
				mu->last_t = strtoul(&s[11], NULL, 16);
				mu->usages = strtoul(&s[20], NULL, 16);
				mu->flags = strtoul(&s[29], NULL, 16);
				debug(D_REGISTRY, "Registry loaded machine URL '%s', machine '%s', first: %u, last: %u, usages: %u, flags: %02x", u->url, m->guid, mu->first_t, mu->last_t, mu->usages, mu->flags);
				break;

			default:
				error("Registry: ignoring line %zu of filename '%s': %s.", line, registry.db_filename, s);
				break;
		}
	}
	fclose(fp);

	registry_log_load();

	return line;
}

// ----------------------------------------------------------------------------
// REGISTRY

void registry_init(void) {
	char filename[FILENAME_MAX + 1];

	registry.pathname = config_get("registry", "registry db directory", VARLIB_DIR);
	if(mkdir(registry.pathname, 0644) == -1 && errno != EEXIST)
		error("Cannot create directory '%s'", registry.pathname);

	snprintf(filename, FILENAME_MAX, "%s/%s", registry.pathname, "registry.db");
	registry.db_filename = config_get("registry", "registry db file", filename);

	snprintf(filename, FILENAME_MAX, "%s/%s", registry.pathname, "registry-log.db");
	registry.log_filename = config_get("registry", "registry log file", filename);

	registry.persons_count = 0;
	registry.machines_count = 0;
	registry.usages_count = 0;
	registry.urls_count = 0;
	registry.persons_urls_count = 0;
	registry.machines_urls_count = 0;

	debug(D_REGISTRY, "Registry: creating global registry dictionary for persons.");
	registry.persons = dictionary_create(DICTIONARY_FLAGS);

	debug(D_REGISTRY, "Registry: creating global registry dictionary for machines.");
	registry.machines = dictionary_create(DICTIONARY_FLAGS);

	debug(D_REGISTRY, "Registry: creating global registry dictionary for urls.");
	registry.urls = dictionary_create(DICTIONARY_FLAGS);

	registry_log_open();
	registry_load();
}

void registry_free(void) {
	while(registry.persons->values_index.root) {
		PERSON *p = ((NAME_VALUE *)registry.persons->values_index.root)->value;

		// fprintf(stderr, "\nPERSON: '%s', first: %u, last: %u, usages: %u\n", p->guid, p->first_t, p->last_t, p->usages);

		while(p->urls->values_index.root) {
			PERSON_URL *pu = ((NAME_VALUE *)p->urls->values_index.root)->value;

			// fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", pu->url->url, pu->first_t, pu->last_t, pu->usages, pu->flags);

			debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", pu->url->url, p->guid);
			dictionary_del(p->urls, pu->url->url);

			debug(D_REGISTRY, "Registry: unlinking url '%s' from person", pu->url->url);
			registry_url_unlink(pu->url);

			debug(D_REGISTRY, "Registry: freeing person url");
			free(pu);
		}

		debug(D_REGISTRY, "Registry: deleting person '%s' from persons registry", p->guid);
		dictionary_del(registry.persons, p->guid);

		debug(D_REGISTRY, "Registry: destroying URL dictionary of person '%s'", p->guid);
		dictionary_destroy(p->urls);

		debug(D_REGISTRY, "Registry: freeing person '%s'", p->guid);
		free(p);
	}

	while(registry.machines->values_index.root) {
		MACHINE *m = ((NAME_VALUE *)registry.machines->values_index.root)->value;

		// fprintf(stderr, "\nMACHINE: '%s', first: %u, last: %u, usages: %u\n", m->guid, m->first_t, m->last_t, m->usages);

		while(m->urls->values_index.root) {
			MACHINE_URL *mu = ((NAME_VALUE *)m->urls->values_index.root)->value;

			// fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", mu->url->url, mu->first_t, mu->last_t, mu->usages, mu->flags);

			//debug(D_REGISTRY, "Registry: destroying persons dictionary from url '%s'", mu->url->url);
			//dictionary_destroy(mu->persons);

			debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", mu->url->url, m->guid);
			dictionary_del(m->urls, mu->url->url);

			debug(D_REGISTRY, "Registry: unlinking url '%s' from machine", mu->url->url);
			registry_url_unlink(mu->url);

			debug(D_REGISTRY, "Registry: freeing machine url");
			free(mu);
		}

		debug(D_REGISTRY, "Registry: deleting machine '%s' from machines registry", m->guid);
		dictionary_del(registry.machines, m->guid);

		debug(D_REGISTRY, "Registry: destroying URL dictionary of machine '%s'", m->guid);
		dictionary_destroy(m->urls);

		debug(D_REGISTRY, "Registry: freeing machine '%s'", m->guid);
		free(m);
	}

	debug(D_REGISTRY, "Registry: destroying persons dictionary");
	dictionary_destroy(registry.persons);

	debug(D_REGISTRY, "Registry: destroying machines dictionary");
	dictionary_destroy(registry.machines);

	debug(D_REGISTRY, "Registry: destroying urls dictionary");
	dictionary_destroy(registry.urls);
}


// ----------------------------------------------------------------------------
// TESTS

int test1(int argc, char **argv) {

	void print_stats(uint32_t requests, unsigned long long start, unsigned long long end) {
		fprintf(stderr, " > SPEED: %u requests served in %0.2f seconds ( >>> %llu per second <<< )\n",
				requests, (end-start) / 1000000.0, (unsigned long long)requests * 1000000ULL / (end-start));

		fprintf(stderr, " > DB   : persons %llu, machines %llu, unique URLs %llu, accesses %llu, URLs: for persons %llu, for machines %llu\n",
				registry.persons_count, registry.machines_count, registry.urls_count, registry.usages_count,
				registry.persons_urls_count, registry.machines_urls_count);
	}

	(void) argc;
	(void) argv;

	uint32_t u, users = 1000000;
	uint32_t m, machines = 200000;
	uint32_t machines2 = machines * 2;

	char **users_guids = malloc(users * sizeof(char *));
	char **machines_guids = malloc(machines2 * sizeof(char *));
	char **machines_urls = malloc(machines2 * sizeof(char *));
	unsigned long long start;

	registry_init();

	fprintf(stderr, "Generating %u machine guids\n", machines2);
	for(m = 0; m < machines2 ;m++) {
		uuid_t uuid;
		machines_guids[m] = malloc(36+1);
		uuid_generate(uuid);
		uuid_unparse(uuid, machines_guids[m]);

		char buf[FILENAME_MAX + 1];
		snprintf(buf, FILENAME_MAX, "http://%u.netdata.rocks/", m+1);
		machines_urls[m] = strdup(buf);

		// fprintf(stderr, "\tmachine %u: '%s', url: '%s'\n", m + 1, machines_guids[m], machines_urls[m]);
	}

	start = timems();
	fprintf(stderr, "\nGenerating %u users accessing %u machines\n", users, machines);
	m = 0;
	time_t now = time(NULL);
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(NULL, machines_guids[m], machines_urls[m], now);
		users_guids[u] = p->guid;
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\nAll %u users accessing again the same %u servers\n", users, machines);
	m = 0;
	now = time(NULL);
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(users_guids[u], machines_guids[m], machines_urls[m], now);

		if(p->guid != users_guids[u])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[u], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\nAll %u users accessing a new server, out of the %u servers\n", users, machines);
	m = 1;
	now = time(NULL);
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(users_guids[u], machines_guids[m], machines_urls[m], now);

		if(p->guid != users_guids[u])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[u], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\n%u random users accessing a random server, out of the %u servers\n", users, machines);
	now = time(NULL);
	for(u = 0; u < users ; u++) {
		uint32_t tu = random() * users / RAND_MAX;
		uint32_t tm = random() * machines / RAND_MAX;

		PERSON *p = registry_request(users_guids[tu], machines_guids[tm], machines_urls[tm], now);

		if(p->guid != users_guids[tu])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\n%u random users accessing a random server, out of %u servers\n", users, machines2);
	now = time(NULL);
	for(u = 0; u < users ; u++) {
		uint32_t tu = random() * users / RAND_MAX;
		uint32_t tm = random() * machines2 / RAND_MAX;

		PERSON *p = registry_request(users_guids[tu], machines_guids[tm], machines_urls[tm], now);

		if(p->guid != users_guids[tu])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
	}
	print_stats(u, start, timems());

	for(m = 0; m < 10; m++) {
		start = timems();
		fprintf(stderr,
				"\n%u random user accesses to a random server, out of %u servers,\n > using 1/10000 with a random url, 1/1000 with a mismatched url\n",
				users * 2, machines2);
		now = time(NULL);
		for (u = 0; u < users * 2; u++) {
			uint32_t tu = random() * users / RAND_MAX;
			uint32_t tm = random() * machines2 / RAND_MAX;

			char *url = machines_urls[tm];
			char buf[FILENAME_MAX + 1];
			if (random() % 10000 == 1234) {
				snprintf(buf, FILENAME_MAX, "http://random.%ld.netdata.rocks/", random());
				url = buf;
			}
			else if (random() % 1000 == 123)
				url = machines_urls[random() * machines2 / RAND_MAX];

			PERSON *p = registry_request(users_guids[tu], machines_guids[tm], url, now);

			if (p->guid != users_guids[tu])
				fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
		}
		print_stats(u, start, timems());
	}

	fprintf(stderr, "\n\nSAVE\n");
	start = timems();
	registry_save();
	print_stats(registry.persons_count, start, timems());

	fprintf(stderr, "\n\nCLEANUP\n");
	start = timems();
	registry_free();
	print_stats(registry.persons_count, start, timems());
	return 0;
}

// ----------------------------------------------------------------------------
// TESTING

int main(int argc, char **argv) {
	debug_flags = 0xFFFFFFFF;
	//test1(argc, argv);
	//exit(0);

	(void)argc;
	(void)argv;


	PERSON *p1, *p2;

	fprintf(stderr, "\n\nINITIALIZATION\n");

	registry_init();

	int i = 2;

	fprintf(stderr, "\n\nADDING ENTRY\n");
	// p1 = registry_request("2c95abd0-1542-11e6-8c66-00508db7e9c9", "7c173980-145c-11e6-b86f-00508db7e9c1", "http://localhost:19999/", time(NULL));

	if(0)
	while(i--) {
#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ENTRY\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(NULL, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://localhost:19999/", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://127.0.0.1:19999/", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER PERSON\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request(NULL, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request(p2->guid, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/", time(NULL));
	}

	fprintf(stderr, "\n\nSAVE\n");
	registry_save();

	fprintf(stderr, "\n\nCLEANUP\n");
	registry_free();
	return 0;
}
