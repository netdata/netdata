#ifdef HAVE_CONFIG_H
#include <config.h>
#endif

// gcc -O3 -Wall -Wextra -I ../src/ -I ../ -o registry ../src/registry.c ../src/dictionary.o ../src/log.o ../src/avl.o ../src/common.o -pthread -luuid -DHAVE_CONFIG_H

#include <uuid/uuid.h>
#include <inttypes.h>
#include <stdlib.h>
#include <string.h>

#include "log.h"
#include "common.h"
#include "dictionary.h"

struct machine {
	char guid[36 + 1];

	time_t first_t;
	time_t last_t;
	size_t usages;
	
	DICTIONARY *urls;
};
typedef struct machine MACHINE;


struct person {
	char guid[36 + 1];

	time_t first_t;
	time_t last_t;
	size_t usages;

	DICTIONARY *urls;
};
typedef struct person PERSON;


struct url {
	MACHINE *machine;
	PERSON *person;

	time_t first_t;
	time_t last_t;
	size_t usages;

	size_t url_length;
	char url[];
};
typedef struct url URL;


struct registry {
	DICTIONARY *persons;
	DICTIONARY *machines;
} registry;


// ----------------------------------------------------------------------------
// MACHINE

MACHINE *registry_machine_load(const char *machine_guid) {
	(void)machine_guid;

	return NULL;
}

int registry_machine_save(MACHINE *m) {
	(void)m;

	return -1;
}

MACHINE *registry_machine_find(const char *machine_guid) {
	MACHINE *m = dictionary_get(registry.machines, machine_guid);
	if(!m) m = registry_machine_load(machine_guid);
	return m;
}


MACHINE *registry_machine_get(const char *machine_guid) {
	MACHINE *m = registry_machine_find(machine_guid);
	if(!m) {
		debug(D_REGISTRY, "Registry: creating new machine '%s'", machine_guid);

		m = calloc(1, sizeof(MACHINE));
		if(!m) fatal("Registry: cannot allocate memory for new machine '%s'", machine_guid);

		strncpy(m->guid, machine_guid, 36);

		dictionary_set(registry.machines, m->guid, m, sizeof(MACHINE));
		
		m->first_t = time(NULL);
		m->usages = 0;
	}

	return m;
}


// ----------------------------------------------------------------------------
// PERSON

PERSON *registry_person_load(const char *person_guid) {
	(void)person_guid;

	return NULL;
}

int registry_person_save(PERSON *p) {
	(void)p;

	return -1;
}

PERSON *registry_person_find(const char *person_guid) {
	PERSON *p = dictionary_get(registry.persons, person_guid);
	if(!p) p = registry_person_load(person_guid);
	return p;
}

PERSON *registry_person_get(const char *person_guid) {
	PERSON *p = NULL;

	if(person_guid && *person_guid)
		p = registry_person_find(person_guid);

	if(!p) {
		if(person_guid && *person_guid)
			error("Registry: discarding unknown person guid '%s'. Will give a new PERSONID.", person_guid);

		debug(D_REGISTRY, "Registry: creating new person");

		p = calloc(1, sizeof(PERSON));
		if(!p) fatal("Registry: cannot allocate memory for new person.");

		uuid_t uuid;
		if(uuid_generate_time_safe(uuid) == -1)
			info("Registry: uuid_generate_time_safe() reports UUID generation is not safe for uniqueness.");

		uuid_unparse_lower(uuid, p->guid);

		dictionary_set(registry.persons, p->guid, p, sizeof(PERSON));

		p->first_t = time(NULL);
		p->usages = 0;
	}

	return p;
}


// ----------------------------------------------------------------------------
// URL

URL *registry_url_update(PERSON *p, MACHINE *m, const char *url) {
	URL *pu = dictionary_get(p->urls, url);
	URL *mu = dictionary_get(m->urls, url);
	URL *u = NULL;

	if(pu != mu || pu == NULL || mu == NULL)
		error("Registry: person/machine discrepancy on url '%s', for person '%s' and machine '%s'", url, p->guid, m->guid);
	else
		u = pu;

	if(!u) {
		size_t len = strlen(url);

		URL *u = calloc(1, sizeof(URL) + len);
		if(!u) fatal("Registry: cannot allocate memory for person '%s', machine '%s', url '%s'.", p->guid, m->guid, url);

		strcpy(u->url, url);
		u->url_length = len;
		u->person = p;
		u->machine = m;
		u->first_t = time(NULL);
		u->usages = 1;

		dictionary_set(p->urls, url, u, sizeof(URL) + u->url_length);
		dictionary_set(m->urls, url, u, sizeof(URL) + u->url_length);
		if(pu) free(pu);
		if(mu) free(mu);
	}
	else
		u->usages++;

	p->usages++;
	m->usages++;
	p->last_t = m->last_t = u->last_t = time(NULL);

	return u;
}


// ----------------------------------------------------------------------------
// REGISTRY

int registry_save(void) {
	return -1;
}

char *registry_request(const char *person_guid, const char *machine_guid, const char *url) {
	PERSON *p = NULL;
	MACHINE *m = NULL;

	// --- PERSON ---
	p = registry_person_get(person_guid);
	person_guid = p->guid;

	// --- MACHINE ---
	m = registry_machine_get(machine_guid);
	machine_guid = m->guid;

	// --- URL ---
	registry_url_update(p, m, url);

	registry_person_save(p);
	registry_machine_save(m);
	registry_save();

	return NULL;
}

void registry_init(void) {
	registry.persons = dictionary_create(DICTIONARY_FLAG_DEFAULT);
	if(!registry.persons)
		fatal("Registry: cannot create persons registry");

	registry.machines = dictionary_create(DICTIONARY_FLAG_DEFAULT);
	if(!registry.machines)
		fatal("Registry: cannot create machines registry");
}

int main(int argc, char **argv) {
	(void)argc;
	(void)argv;

	registry_init();

	return 0;
}
