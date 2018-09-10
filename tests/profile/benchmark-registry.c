/* SPDX-License-Identifier: GPL-3.0+ */

/*
 * compile with
 *  gcc -O1 -ggdb -Wall -Wextra -I ../src/ -I ../ -o benchmark-registry benchmark-registry.c ../src/dictionary.o ../src/log.o ../src/avl.o ../src/common.o ../src/appconfig.o ../src/web_buffer.o ../src/storage_number.o ../src/rrd.o ../src/health.o -pthread -luuid -lm -DHAVE_CONFIG_H -DVARLIB_DIR="\"/tmp\""
 */

char *hostname = "me";

#include "../src/registry.c"

void netdata_cleanup_and_exit(int ret) { exit(ret); }

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
		snprintfz(buf, FILENAME_MAX, "http://%u.netdata.rocks/", m+1);
		machines_urls[m] = strdup(buf);

		// fprintf(stderr, "\tmachine %u: '%s', url: '%s'\n", m + 1, machines_guids[m], machines_urls[m]);
	}

	start = timems();
	fprintf(stderr, "\nGenerating %u users accessing %u machines\n", users, machines);
	m = 0;
	time_t now = time(NULL);
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request_access(NULL, machines_guids[m], machines_urls[m], "test", now);
		users_guids[u] = p->guid;
	}
	print_stats(u, start, timems());

	start = timems();
	fprintf(stderr, "\nAll %u users accessing again the same %u servers\n", users, machines);
	m = 0;
	now = time(NULL);
	for(u = 0; u < users ; u++) {
		if(++m == machines) m = 0;

		PERSON *p = registry_request_access(users_guids[u], machines_guids[m], machines_urls[m], "test", now);

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

		PERSON *p = registry_request_access(users_guids[u], machines_guids[m], machines_urls[m], "test", now);

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

		PERSON *p = registry_request_access(users_guids[tu], machines_guids[tm], machines_urls[tm], "test", now);

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

		PERSON *p = registry_request_access(users_guids[tu], machines_guids[tm], machines_urls[tm], "test", now);

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
				snprintfz(buf, FILENAME_MAX, "http://random.%ld.netdata.rocks/", random());
				url = buf;
			}
			else if (random() % 1000 == 123)
				url = machines_urls[random() * machines2 / RAND_MAX];

			PERSON *p = registry_request_access(users_guids[tu], machines_guids[tm], url, "test", now);

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
	config_set_boolean("registry", "enabled", 1);

	//debug_flags = 0xFFFFFFFF;
	test1(argc, argv);
	exit(0);

	(void)argc;
	(void)argv;


	PERSON *p1, *p2;

	fprintf(stderr, "\n\nINITIALIZATION\n");

	registry_init();

	int i = 2;

	fprintf(stderr, "\n\nADDING ENTRY\n");
	p1 = registry_request_access("2c95abd0-1542-11e6-8c66-00508db7e9c9", "7c173980-145c-11e6-b86f-00508db7e9c1", "http://localhost:19999/", "test", time(NULL));

	if(0)
	while(i--) {
#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ENTRY\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request_access(NULL, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://localhost:19999/", "test", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request_access(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://127.0.0.1:19999/", "test", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER URL\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request_access(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/", "test", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p1 = registry_request_access(p1->guid, "7c173980-145c-11e6-b86f-00508db7e9c1", "http://my.server:19999/", "test", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER PERSON\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request_access(NULL, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/", "test", time(NULL));

#ifdef REGISTRY_STDOUT_DUMP
		fprintf(stderr, "\n\nADDING ANOTHER MACHINE\n");
#endif /* REGISTRY_STDOUT_DUMP */
		p2 = registry_request_access(p2->guid, "7c173980-145c-11e6-b86f-00508db7e9c3", "http://localhost:19999/", "test", time(NULL));
	}

	fprintf(stderr, "\n\nSAVE\n");
	registry_save();

	fprintf(stderr, "\n\nCLEANUP\n");
	registry_free();
	return 0;
}
