#include <pthread.h>
#include <stdlib.h>
#include <string.h>

#include "common.h"
#include "config.h"
#include "log.h"

#define CONFIG_FILE_LINE_MAX 4096

pthread_rwlock_t config_rwlock = PTHREAD_RWLOCK_INITIALIZER;

struct config_value {
	char name[CONFIG_MAX_NAME + 1];
	char value[CONFIG_MAX_VALUE + 1];

	unsigned long hash;		// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons

	int loaded;				// loaded from the user config
	int used;				// has been accessed from the program
	int changed;			// changed from the internal default

	struct config_value *next;
};

struct config {
	char name[CONFIG_MAX_NAME + 1];

	unsigned long hash;		// a simple hash to speed up searching
							// we first compare hashes, and only if the hashes are equal we do string comparisons

	struct config_value *values;

	struct config *next;
} *config_root = NULL;

struct config_value *config_value_create(struct config *co, const char *name, const char *value)
{
	debug(D_CONFIG, "Creating config entry for name '%s', value '%s', in section '%s'.", name, value, co->name);

	struct config_value *cv = calloc(1, sizeof(struct config_value));
	if(!cv) fatal("Cannot allocate config_value");

	strncpy(cv->name,  name,  CONFIG_MAX_NAME);
	strncpy(cv->value, value, CONFIG_MAX_VALUE);
	cv->hash = simple_hash(cv->name);

	// no need for string termination, due to calloc()

	struct config_value *cv2 = co->values;
	if(cv2) {
		while (cv2->next) cv2 = cv2->next;
		cv2->next = cv;
	}
	else co->values = cv;

	return cv;
}

struct config *config_create(const char *section)
{
	debug(D_CONFIG, "Creating section '%s'.", section);

	struct config *co = calloc(1, sizeof(struct config));
	if(!co) fatal("Cannot allocate config");

	strncpy(co->name, section, CONFIG_MAX_NAME);
	co->hash = simple_hash(co->name);

	// no need for string termination, due to calloc()

	struct config *co2 = config_root;
	if(co2) {
		while (co2->next) co2 = co2->next;
		co2->next = co;
	}
	else config_root = co;

	return co;
}

struct config *config_find_section(const char *section)
{
	struct config *co;
	unsigned long hash = simple_hash(section);

	for(co = config_root; co ; co = co->next)
		if(hash == co->hash)
			if(strcmp(co->name, section) == 0)
				break;

	return co;
}

int load_config(char *filename, int overwrite_used)
{
	int line = 0;
	struct config *co = NULL;

	pthread_rwlock_wrlock(&config_rwlock);

	char buffer[CONFIG_FILE_LINE_MAX + 1], *s;

	if(!filename) filename = CONFIG_DIR "/" CONFIG_FILENAME;
	FILE *fp = fopen(filename, "r");
	if(!fp) {
		error("Cannot open file '%s'", CONFIG_DIR "/" CONFIG_FILENAME);
		pthread_rwlock_unlock(&config_rwlock);
		return 0;
	}

	while(fgets(buffer, CONFIG_FILE_LINE_MAX, fp) != NULL) {
		buffer[CONFIG_FILE_LINE_MAX] = '\0';
		line++;

		s = trim(buffer);
		if(!s) {
			debug(D_CONFIG, "Ignoring line %d, it is empty.", line);
			continue;
		}

		int len = strlen(s);
		if(*s == '[' && s[len - 1] == ']') {
			// new section
			s[len - 1] = '\0';
			s++;

			co = config_find_section(s);
			if(!co) co = config_create(s);

			continue;
		}

		if(!co) {
			// line outside a section
			error("Ignoring line %d ('%s'), it is outsize all sections.", line, s);
			continue;
		}

		char *name = s;
		char *value = strchr(s, '=');
		if(!value) {
			error("Ignoring line %d ('%s'), there is no = in it.", line, s);
			continue;
		}
		*value = '\0';
		value++;

		name = trim(name);
		value = trim(value);

		if(!name) {
			error("Ignoring line %d, name is empty.", line);
			continue;
		}
		if(!value) {
			debug(D_CONFIG, "Ignoring line %d, value is empty.", line);
			continue;
		}

		struct config_value *cv;
		for(cv = co->values; cv ; cv = cv->next)
			if(strcmp(cv->name, name) == 0) break;

		if(!cv) cv = config_value_create(co, name, value);
		else {
			if((cv->used && overwrite_used) || !cv->used) {
				debug(D_CONFIG, "Overwriting '%s/%s'.", line, co->name, cv->name);
				strncpy(cv->value, value, CONFIG_MAX_VALUE);
				// termination is already there
			}
			else
				debug(D_CONFIG, "Ignoring line %d, '%s/%s' is already present and used.", line, co->name, cv->name);
		}
		cv->loaded = 1;
	}

	fclose(fp);

	pthread_rwlock_unlock(&config_rwlock);
	return 1;
}

char *config_get(const char *section, const char *name, const char *default_value)
{
	struct config_value *cv;

	debug(D_CONFIG, "request to get config in section '%s', name '%s', default_value '%s'", section, name, default_value);

	pthread_rwlock_rdlock(&config_rwlock);

	struct config *co = config_find_section(section);
	if(!co) co = config_create(section);

	unsigned long hash = simple_hash(name);
	for(cv = co->values; cv ; cv = cv->next)
		if(hash == cv->hash)
			if(strcmp(cv->name, name) == 0)
				break;

	if(!cv) cv = config_value_create(co, name, default_value);
	cv->used = 1;

	if(cv->loaded || cv->changed) {
		// this is a loaded value from the config file
		// if it is different that the default, mark it
		if(strcmp(cv->value, default_value) != 0) cv->changed = 1;
	}
	else {
		// this is not loaded from the config
		// copy the default value to it
		strncpy(cv->value, default_value, CONFIG_MAX_VALUE);
	}

	pthread_rwlock_unlock(&config_rwlock);
	return(cv->value);
}

long long config_get_number(const char *section, const char *name, long long value)
{
	char buffer[100], *s;
	sprintf(buffer, "%lld", value);

	s = config_get(section, name, buffer);
	return strtoll(s, NULL, 0);
}

int config_get_boolean(const char *section, const char *name, int value)
{
	char *s;
	if(value) s = "yes";
	else s = "no";

	s = config_get(section, name, s);

	if(strcmp(s, "yes") == 0 || strcmp(s, "true") == 0 || strcmp(s, "1") == 0) {
		strcpy(s, "yes");
		return 1;
	}
	else {
		strcpy(s, "no");
		return 0;
	}
}

const char *config_set(const char *section, const char *name, const char *value)
{
	struct config_value *cv;

	debug(D_CONFIG, "request to set config in section '%s', name '%s', value '%s'", section, name, value);

	pthread_rwlock_wrlock(&config_rwlock);

	struct config *co = config_find_section(section);
	if(!co) co = config_create(section);

	unsigned long hash = simple_hash(name);
	for(cv = co->values; cv ; cv = cv->next)
		if(hash == cv->hash)
			if(strcmp(cv->name, name) == 0)
				break;

	if(!cv) cv = config_value_create(co, name, value);
	cv->used = 1;

	if(strcmp(cv->value, value) != 0) cv->changed = 1;

	strncpy(cv->value, value, CONFIG_MAX_VALUE);
	// termination is already there

	pthread_rwlock_unlock(&config_rwlock);

	return value;
}

long long config_set_number(const char *section, const char *name, long long value)
{
	char buffer[100];
	sprintf(buffer, "%lld", value);

	config_set(section, name, buffer);

	return value;
}

int config_set_boolean(const char *section, const char *name, int value)
{
	char *s;
	if(value) s = "yes";
	else s = "no";

	config_set(section, name, s);

	return value;
}

void generate_config(struct web_buffer *wb, int only_changed)
{
	int i, pri;
	struct config *co;
	struct config_value *cv;

	for(i = 0; i < 3 ;i++) {
		web_buffer_increase(wb, 500);
		switch(i) {
			case 0:
				web_buffer_printf(wb, 
					"# NetData Configuration\n"
					"# You can uncomment and change any of the options bellow.\n"
					"# The value shown in the commented settings, is the default value.\n"
					"\n# global netdata configuration\n");
				break;

			case 1:
				web_buffer_printf(wb, "\n\n# per plugin configuration\n");
				break;

			case 2:
				web_buffer_printf(wb, "\n\n# per chart configuration\n");
				break;
		}

		for(co = config_root; co ; co = co->next) {
			if(strcmp(co->name, "global") == 0 || strcmp(co->name, "plugins") == 0) pri = 0;
			else if(strncmp(co->name, "plugin:", 7) == 0) pri = 1;
			else pri = 2;

			if(i == pri) {
				int used = 0;
				int changed = 0;
				int count = 0;
				for(cv = co->values; cv ; cv = cv->next) {
					used += cv->used;
					changed += cv->changed;
					count++;
				}

				if(!count) continue;
				if(only_changed && !changed) continue;

				if(!used) {
					web_buffer_increase(wb, 500);
					web_buffer_printf(wb, "\n# node '%s' is not used.", co->name);
				}

				web_buffer_increase(wb, CONFIG_MAX_NAME + 4);
				web_buffer_printf(wb, "\n[%s]\n", co->name);

				for(cv = co->values; cv ; cv = cv->next) {

					if(used && !cv->used) {
						web_buffer_increase(wb, CONFIG_MAX_NAME + 200);
						web_buffer_printf(wb, "\n\t# option '%s' is not used.\n", cv->name);
					}
					web_buffer_increase(wb, CONFIG_MAX_NAME + CONFIG_MAX_VALUE + 5);
					web_buffer_printf(wb, "\t%s%s = %s\n", (!cv->changed && cv->used)?"# ":"", cv->name, cv->value);
				}
			}
		}
	}
}

