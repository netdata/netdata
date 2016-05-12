#include "web_client.h"

#ifndef NETDATA_REGISTRY_H
#define NETDATA_REGISTRY_H 1

#define NETDATA_REGISTRY_COOKIE_NAME "netdata_registry_id"

extern int registry_request_access_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when);
extern int registry_request_delete_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
extern int registry_request_search_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);
extern int registry_request_switch_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *new_person_guid, time_t when);
extern int registry_request_hello_json(struct web_client *w);

extern int registry_init(void);
extern void registry_free(void);
extern int registry_save(void);

extern char *registry_get_this_machine_guid(void);

extern void registry_statistics(void);


#endif /* NETDATA_REGISTRY_H */
