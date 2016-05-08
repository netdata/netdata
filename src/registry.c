#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

// gcc -O1 -ggdb -Wall -Wextra -I ../src/ -I ../ -o registry ../src/registry.c ../src/dictionary.o ../src/log.o ../src/avl.o ../src/common.o -pthread -luuid -DHAVE_CONFIG_H

#include <uuid/uuid.h>
#include <inttypes.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "common.h"
#include "dictionary.h"

#define REGISTRY_MEMORY_ATTRIBUTE aligned

#define REGISTRY_URL_FLAGS_DEFAULT 0x00
#define REGISTRY_URL_FLAGS_EXPIRED 0x01

// ----------------------------------------------------------------------------
// COMMON structures

struct registry {
	unsigned long long persons_count;
	unsigned long long machines_count;
	unsigned long long usages_count;
	unsigned long long urls_count;
	unsigned long long persons_urls_count;
	unsigned long long machines_urls_count;

	DICTIONARY *persons; 	// dictionary of PERSON *, with key the PERSON.guid
	DICTIONARY *machines; 	// dictionary of MACHINE *, with key the MACHINE.guid
	DICTIONARY *urls; 		// dictionary of URL *, with key the URL.url
} registry;


// ----------------------------------------------------------------------------
// URL structures
// Save memory by de-duplicating URLs

struct url {
	uint32_t links;
	uint16_t url_length;
	char url[];
}  __attribute__ (( REGISTRY_MEMORY_ATTRIBUTE ));
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
}  __attribute__ (( REGISTRY_MEMORY_ATTRIBUTE ));
typedef struct machine_url MACHINE_URL;

// A machine
struct machine {
	char guid[36 + 1];

	DICTIONARY *urls; 			// MACHINE_URL *

	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
} __attribute__ (( REGISTRY_MEMORY_ATTRIBUTE ));
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
} __attribute__ (( REGISTRY_MEMORY_ATTRIBUTE ));
typedef struct person_url PERSON_URL;

// A person
struct person {
	char guid[36 + 1];

	DICTIONARY *urls; // PERSON_URL *

	uint32_t first_t;			// the first time we saw this
	uint32_t last_t;			// the last time we saw this
	uint32_t usages;			// how many times this has been accessed
} __attribute__ (( REGISTRY_MEMORY_ATTRIBUTE ));
typedef struct person PERSON;


// ----------------------------------------------------------------------------
// URL

static inline URL *registry_url_get(const char *url) {
	debug(D_DICTIONARY, "Registry: registry_url_get('%s')", url);
	URL *u = dictionary_get(registry.urls, url);
	if(!u) {
		size_t len = strlen(url);

		debug(D_DICTIONARY, "Registry: registry_url_get('%s'): allocating %zu bytes", url, sizeof(URL) + len + 1);
		u = malloc(sizeof(URL) + len + 1);
		if(!u) fatal("Cannot allocate %zu bytes for URL '%s'", sizeof(URL) + len + 1, url);

		strcpy(u->url, url);
		u->links = 0;
		u->url_length = len;

		debug(D_DICTIONARY, "Registry: registry_url_get('%s'): indexing it", url);
		dictionary_set(registry.urls, url, u, sizeof(URL) + len + 1);

		registry.urls_count++;
	}

	return u;
}

static inline void registry_url_link(URL *u) {
	u->links++;
}

static inline void registry_url_unlink(URL *u) {
	u->links--;
	if(!u->links) {
		dictionary_del(registry.urls, u->url);
		free(u);
	}
}


// ----------------------------------------------------------------------------
// MACHINE

int dump_machine_url(void *entry, void *nothing) {
	(void)nothing;

	MACHINE_URL *mu = entry;

	fprintf(stderr, "\n\tURL '%s'\n\t\tfirst seen %u, last seen %u, usages %u, flags 0x%02x\n",
			mu->url->url,
			mu->first_t,
			mu->last_t,
			mu->usages,
			mu->flags
	);

	return 0;
}

int dump_machine(void *entry, void *nothing) {
	(void)nothing;

	MACHINE *m = entry;

	fprintf(stderr, "MACHINE '%s'\n\tfirst seen %u, last seen %u, usages %u\n",
			m->guid,
			(uint32_t)m->first_t,
			(uint32_t)m->last_t,
			m->usages
	);

	dictionary_get_all(m->urls, dump_machine_url, NULL);
	return 0;
}

MACHINE *registry_machine_load(const char *machine_guid) {
	debug(D_REGISTRY, "Registry: registry_machine_load('%s')", machine_guid);
	(void)machine_guid;

	debug(D_REGISTRY, "Registry: registry_machine_load('%s'): not found", machine_guid);
	return NULL;
}

int registry_machine_save(MACHINE *m) {
	debug(D_REGISTRY, "Registry: registry_machine_save('%s')", m->guid);

#ifdef REGISTRY_STDOUT_DUMP
	fprintf(stderr, "\nSAVING ");
	dump_machine(m, NULL);
#endif /* REGISTRY_STDOUT_DUMP */

	return -1;
}

MACHINE *registry_machine_find(const char *machine_guid) {
	debug(D_REGISTRY, "Registry: registry_machine_find('%s')", machine_guid);

	MACHINE *m = dictionary_get(registry.machines, machine_guid);
	if(!m) m = registry_machine_load(machine_guid);
	return m;
}


MACHINE *registry_machine_allocate(const char *machine_guid) {
	debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating new machine, sizeof(MACHINE)=%zu", machine_guid, sizeof(MACHINE));

	MACHINE *m = calloc(1, sizeof(MACHINE));
	if(!m) fatal("Registry: cannot allocate memory for new machine '%s'", machine_guid);

	strncpy(m->guid, machine_guid, 36);

	debug(D_REGISTRY, "Registry: registry_machine_allocate('%s'): creating dictionary of urls", machine_guid);
	m->urls = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);

	m->first_t = m->last_t = time(NULL);
	m->usages = 0;

	dictionary_set(registry.machines, m->guid, m, sizeof(MACHINE));

	return m;
}

MACHINE *registry_machine_get(const char *machine_guid) {
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
				m = registry_machine_allocate(machine_guid);
				registry.machines_count++;
			}
		}
	}

	return m;
}


// ----------------------------------------------------------------------------
// PERSON

int dump_person_url(void *entry, void *nothing) {
	(void)nothing;

	PERSON_URL *pu = entry;

	fprintf(stderr, "\n\tURL '%s'\n\t\tfirst seen %u, last seen %u, usages %u, flags 0x%02x\n",
			pu->url->url,
			pu->first_t,
			pu->last_t,
			pu->usages,
			pu->flags
	);

	return 0;
}

int dump_person(void *entry, void *nothing) {
	(void)nothing;

	PERSON *p = entry;

	fprintf(stderr, "PERSON '%s'\n\tfirst seen %u, last seen %u, usages %u\n",
			p->guid,
			p->first_t,
			p->last_t,
			p->usages
	);

	dictionary_get_all(p->urls, dump_person_url, NULL);
	return 0;
}

int registry_person_save(PERSON *p) {
	debug(D_REGISTRY, "Registry: registry_person_save('%s')", p->guid);

#ifdef REGISTRY_STDOUT_DUMP
	fprintf(stderr, "\nSAVING ");
	dump_person(p, NULL);
#endif /* REGISTRY_STDOUT_DUMP */

	return -1;
}

PERSON *registry_person_load(const char *person_guid) {
	debug(D_REGISTRY, "Registry: registry_person_load('%s')", person_guid);
	(void)person_guid;

	debug(D_REGISTRY, "Registry: registry_person_load('%s'): not found", person_guid);
	return NULL;
}

PERSON *registry_person_find(const char *person_guid) {
	debug(D_REGISTRY, "Registry: registry_person_find('%s')", person_guid);
	PERSON *p = dictionary_get(registry.persons, person_guid);
	if(!p) p = registry_person_load(person_guid);
	return p;
}

PERSON *registry_person_allocate(const char *person_guid) {
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
	p->urls = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);

	p->first_t = p->last_t = time(NULL);
	p->usages = 0;

	dictionary_set(registry.persons, p->guid, p, sizeof(PERSON));
	return p;
}

PERSON *registry_person_get(const char *person_guid) {
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
		p = registry_person_allocate(NULL);
		registry.persons_count++;
	}

	return p;
}


// ----------------------------------------------------------------------------
// LINKING OF OBJECTS

PERSON_URL *registry_person_link_to_url(PERSON *p, MACHINE *m, URL *u) {
	debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): searching for URL in person", p->guid, m->guid, u->url);

	PERSON_URL *pu = dictionary_get(p->urls, u->url);
	if(!pu) {
		debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): not found, allocating %zu bytes", p->guid, m->guid, u->url, sizeof(PERSON_URL));
		pu = malloc(sizeof(PERSON_URL));
		pu->machine = m;
		pu->first_t = pu->last_t = time(NULL);
		pu->usages = 1;
		pu->url = u;
		pu->flags = REGISTRY_URL_FLAGS_DEFAULT;

		debug(D_REGISTRY, "registry_person_link_to_url('%s', '%s', '%s'): indexing URL in person", p->guid, m->guid, u->url);
		dictionary_set(p->urls, u->url, pu, sizeof(PERSON_URL));
		registry_url_link(u);
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

	if(pu->flags & REGISTRY_URL_FLAGS_EXPIRED)
		info("registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL.", p->guid, m->guid, u->url);

	return pu;
}

MACHINE_URL *registry_machine_link_to_url(PERSON *p, MACHINE *m, URL *u) {
	debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): searching for URL in machine", p->guid, m->guid, u->url);

	MACHINE_URL *mu = dictionary_get(m->urls, u->url);
	if(!mu) {
		debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): not found, allocating %zu bytes", p->guid, m->guid, u->url, sizeof(MACHINE_URL));
		mu = malloc(sizeof(MACHINE_URL));
		//mu->persons = dictionary_create(DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);
		mu->first_t = mu->last_t = time(NULL);
		mu->usages = 1;
		mu->url = u;
		mu->flags = REGISTRY_URL_FLAGS_DEFAULT;

		debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): indexing URL in machine", p->guid, m->guid, u->url);
		dictionary_set(m->urls, u->url, mu, sizeof(MACHINE_URL));
		registry_url_link(u);
		registry.machines_urls_count++;
	}
	else {
		debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): found", p->guid, m->guid, u->url);
		mu->usages++;
	}

	//debug(D_REGISTRY, "registry_machine_link_to_url('%s', '%s', '%s'): indexing person in machine", p->guid, m->guid, u->url);
	//dictionary_set(mu->persons, p->guid, p, sizeof(PERSON));

	m->usages++;

	if(mu->flags & REGISTRY_URL_FLAGS_EXPIRED)
		info("registry_person_link_to_url('%s', '%s', '%s'): accessing an expired URL.", p->guid, m->guid, u->url);

	return mu;
}


// ----------------------------------------------------------------------------
// REGISTRY REQUESTS

int registry_save(void) {
	return -1;
}

PERSON *registry_request(const char *person_guid, const char *machine_guid, const char *url) {
	debug(D_REGISTRY, "registry_request('%s', '%s', '%s'): NEW REQUEST", (person_guid)?person_guid:"", machine_guid, url);

	MACHINE *m = registry_machine_get(machine_guid);
	if(!m) return NULL;

	URL *u = registry_url_get(url);

	PERSON *p = registry_person_get(person_guid);
	registry_person_link_to_url(p, m, u);
	registry_machine_link_to_url(p, m, u);

	debug(D_REGISTRY, "Registry: registry_request('%s', '%s', '%s'): saving", person_guid, machine_guid, url);
	registry_person_save(p);
	registry_machine_save(m);

	registry.usages_count++;
	registry_save();

	return p;
}

// ----------------------------------------------------------------------------
// REGISTRY

void registry_init(void) {
	registry.persons_count = 0;
	registry.machines_count = 0;
	registry.usages_count = 0;
	registry.urls_count = 0;
	registry.persons_urls_count = 0;
	registry.machines_urls_count = 0;

	debug(D_REGISTRY, "Registry: creating global registry dictionary for persons.");
	registry.persons = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);

	debug(D_REGISTRY, "Registry: creating global registry dictionary for machines.");
	registry.machines = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);

	debug(D_REGISTRY, "Registry: creating global registry dictionary for urls.");
	registry.urls = dictionary_create(DICTIONARY_FLAG_VALUE_LINK_DONT_CLONE|DICTIONARY_FLAG_NAME_LINK_DONT_CLONE|DICTIONARY_FLAG_SINGLE_THREADED);
}

void registry_free(void) {
	while(registry.persons->values_index.root) {
		PERSON *p = ((NAME_VALUE *)registry.persons->values_index.root)->value;

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\nPERSON: '%s', first: %u, last: %u, usages: %u\n", p->guid, p->first_t, p->last_t, p->usages);
#endif /* REGISTRY_STDOUT_DUMP */

		while(p->urls->values_index.root) {
			PERSON_URL *pu = ((NAME_VALUE *)p->urls->values_index.root)->value;

#ifdef REGISTRY_STDOUT_DUMP
			fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", pu->url->url, pu->first_t, pu->last_t, pu->usages, pu->flags);
#endif /* REGISTRY_STDOUT_DUMP */

			debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", pu->url->url, p->guid);
			dictionary_del(p->urls, pu->url->url);

			debug(D_REGISTRY, "Registry: unlinking url '%s'", pu->url->url);
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

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\nMACHINE: '%s', first: %u, last: %u, usages: %u\n", m->guid, m->first_t, m->last_t, m->usages);
#endif /* REGISTRY_STDOUT_DUMP */

		while(m->urls->values_index.root) {
			MACHINE_URL *mu = ((NAME_VALUE *)m->urls->values_index.root)->value;

#ifdef REGISTRY_STDOUT_DUMP
			fprintf(stderr, "\tURL: '%s', first: %u, last: %u, usages: %u, flags: 0x%02x\n", mu->url->url, mu->first_t, mu->last_t, mu->usages, mu->flags);
#endif /* REGISTRY_STDOUT_DUMP */

			//debug(D_REGISTRY, "Registry: destroying persons dictionary from url '%s'", mu->url->url);
			//dictionary_destroy(mu->persons);

			debug(D_REGISTRY, "Registry: deleting url '%s' from person '%s'", mu->url->url, m->guid);
			dictionary_del(m->urls, mu->url->url);

			debug(D_REGISTRY, "Registry: unlinking url '%s'", mu->url);
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

	uint32_t u, users = 500000;
	uint32_t m, machines = 50000;
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
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(NULL, machines_guids[m], machines_urls[m]);
		users_guids[u] = p->guid;
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\nAll %u users accessing again the same %u servers\n", users, machines);
	m = 0;
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(users_guids[u], machines_guids[m], machines_urls[m]);

		if(p->guid != users_guids[u])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[u], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\nAll %u users accessing a new server, out of the %u servers\n", users, machines);
	m = 1;
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request(users_guids[u], machines_guids[m], machines_urls[m]);

		if(p->guid != users_guids[u])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[u], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\n%u random users accessing a random server, out of the %u servers\n", users, machines);
	for(u = 0; u < users ; u++) {
		uint32_t tu = random() * users / RAND_MAX;
		uint32_t tm = random() * machines / RAND_MAX;

		PERSON *p = registry_request(users_guids[tu], machines_guids[tm], machines_urls[tm]);

		if(p->guid != users_guids[tu])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\n%u random users accessing a random server, out of %u servers\n", users, machines2);
	for(u = 0; u < users ; u++) {
		uint32_t tu = random() * users / RAND_MAX;
		uint32_t tm = random() * machines2 / RAND_MAX;

		PERSON *p = registry_request(users_guids[tu], machines_guids[tm], machines_urls[tm]);

		if(p->guid != users_guids[tu])
			fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
	}
	print_stats(u, start, timems());

	for(m = 0; m < 10; m++) {
		start = timems();
		fprintf(stderr,
				"\n%u random user accesses to a random server, out of %u servers,\n > using 1/10000 with a random url, 1/1000 with a mismatched url\n",
				users * 2, machines2);
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

			PERSON *p = registry_request(users_guids[tu], machines_guids[tm], url);

			if (p->guid != users_guids[tu])
				fprintf(stderr, "ERROR: expected to get user guid '%s' but git '%s'", users_guids[tu], p->guid);
		}
		print_stats(u, start, timems());
	}

	fprintf(stderr, "\n\nCLEANUP\n");
	start = timems();
	registry_free();
	print_stats(registry.persons_count, start, timems());
	return 0;
}

// ----------------------------------------------------------------------------
// TESTING

int main(int argc, char **argv) {
	// debug_flags = 0xFFFFFFFF;
	test1(argc, argv);
	exit(0);

	(void)argc;
	(void)argv;


	PERSON *p1, *p2;

	fprintf(stderr, "\n\nINITIALIZATION\n");

	registry_init();


	int i = 100000;
	while(i--) {
#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ENTRY\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(NULL, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://localhost:19999/");

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://127.0.0.1:19999/");

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/");

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/");

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER PERSON\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request(NULL, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/");

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request(p2->guid, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/");
	}

	fprintf(stderr, "\n\nCLEANUP\n");
	//registry_free();
	return 0;
}
