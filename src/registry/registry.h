// SPDX-License-Identifier: GPL-3.0-or-later
/*
 * netdata registry
 *
 * this header file describes the public interface
 * to the netdata registry
 *
 * only these high level functions are exposed
 *
 */

// ----------------------------------------------------------------------------
// TODO
//
// 1. the default tracking cookie expires in 1 year, but the persons are not
//    removed from the db - this means the database only grows - ideally the
//    database should be cleaned in registry_db_save() for both on-disk and
//    on-memory entries.
//
//    Cleanup:
//    i. Find all the PERSONs that have expired cookie
//    ii. For each of their PERSON_URLs:
//     - decrement the linked MACHINE links
//     - if the linked MACHINE has no other links, remove the linked MACHINE too
//     - remove the PERSON_URL
//
// 2. add protection to prevent abusing the registry by flooding it with
//    requests to fill the memory and crash it.
//
//    Possible protections:
//    - limit the number of URLs per person
//    - limit the number of URLs per machine
//    - limit the number of persons
//    - limit the number of machines
//    - [DONE] limit the size of URLs
//    - [DONE] limit the size of PERSON_URL names
//    - limit the number of requests that add data to the registry,
//      per client IP per hour
//
// 3. lower memory requirements
//
//    - embed avl structures directly into registry objects, instead of DICTIONARY
//      [DONE for PERSON_URLs, PENDING for MACHINE_URLs]
//    - store GUIDs in memory as UUID instead of char *
//    - do not track persons using the demo machines only
//      (i.e. start tracking them only when they access a non-demo machine)
//    - [DONE] do not track custom dashboards by default

#ifndef NETDATA_REGISTRY_H
#define NETDATA_REGISTRY_H 1

#include "../common.h"

#define NETDATA_REGISTRY_COOKIE_NAME "netdata_registry_id"

// initialize the registry
// should only happen when netdata starts
extern int registry_init(void);

// free all data held by the registry
// should only happen when netdata exits
extern void registry_free(void);

// HTTP requests handled by the registry
extern int registry_request_access_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when);
extern int registry_request_delete_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
extern int registry_request_search_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);
extern int registry_request_switch_json(RRDHOST *host, struct web_client *w, char *person_guid, char *machine_guid, char *url, char *new_person_guid, time_t when);
extern int registry_request_hello_json(RRDHOST *host, struct web_client *w);

// update the registry monitoring charts
extern void registry_statistics(void);

extern char *registry_get_this_machine_guid(void);
extern char *registry_get_this_machine_hostname(void);

extern int regenerate_guid(const char *guid, char *result);

#endif /* NETDATA_REGISTRY_H */
