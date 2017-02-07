/**
 * @file registry.h
 * @brief Public interface to the registry.
 *
 * Only these high level functions are exposed.
 *
 * \todo
 * 1. The default tracking cookie expires in 1 year, but the persons are not
 *    removed from the db - this means the database only grows - ideally the
 *    database should be cleaned in `registry_db_save()` for both on-disk and
 *    on-memory entries. \n
 *    Cleanup:
 *    1. Find all the PERSONs that have expired cookie
 *    2. For each of their PERSON_URLs:
 *       - decrement the linked MACHINE links
 *       - if the linked MACHINE has no other links, remove the linked MACHINE too
 *       - remove the PERSON_URL
 * 2. add protection to prevent abusing the registry by flooding it with
 *    requests to fill the memory and crash it.
 *    Possible protections:
 *    - limit the number of URLs per person
 *    - limit the number of URLs per machine
 *    - limit the number of persons
 *    - limit the number of machines
 *    - [DONE] limit the size of URLs
 *    - [DONE] limit the size of PERSON_URL names
 *    - limit the number of requests that add data to the registry,
 *      per client IP per hour
 * 3. lower memory requirements
 *    - embed avl structures directly into registry objects, instead of DICTIONARY
 *      [DONE for PERSON_URLs, PENDING for MACHINE_URLs]
 *    - store GUIDs in memory as UUID instead of char *
 *    - do not track persons using the demo machines only
 *      (i.e. start tracking them only when they access a non-demo machine)
 *    - [DONE] do not track custom dashboards by default
 **/


#ifndef NETDATA_REGISTRY_H
#define NETDATA_REGISTRY_H 1

#define NETDATA_REGISTRY_COOKIE_NAME "netdata_registry_id" ///< registry cookie name.

/**
 * Initialize the registry.
 *
 * Should only happen when netdata starts.
 *
 * @return 0
 */
extern int registry_init(void);

/**
 * Free all data held by the registry.
 *
 * Should only happen when netdata exits.
 */
extern void registry_free(void);

// ----------------------------------------------------------------------------
// HTTP requests handled by the registry

/**
 * Register HTTP access request.
 *
 * Main function for registering an access.
 *
 * @param w Requesting client.
 * @param person_guid of requesting person
 * @param machine_guid of requesting machine
 * @param url requested
 * @param name supplied by the client
 * @param when the request started
 * @return HTTP status code. 200 on success.
 */
extern int registry_request_access_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *name, time_t when);
/**
 * Delete URL from person in registry.
 *
 * the main method for deleting a URL from a person
 *
 * @param w Requesting client.
 * @param person_guid of requesting person
 * @param machine_guid of requesting machine
 * @param url requested
 * @param delete_url url to delete
 * @param when the request started
 * @return HTTP status code. 200 on success.
 */
extern int registry_request_delete_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *delete_url, time_t when);
/**
 * Search URLs of a person in registry.
 *
 * The main method for searching the URLs in registry
 *
 * @param w Requesting client.
 * @param person_guid of requesting person
 * @param machine_guid of requesting machine
 * @param url requested
 * @param request_machine machine which URL is requested
 * @param when the request started
 * @return HTTP status code. 200 on success.
 */extern int registry_request_search_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *request_machine, time_t when);
 /**
 * Switch user identity.
 *
 * The main method for switching user identity.
 *
 * @param w Requesting client.
 * @param person_guid of requesting person
 * @param machine_guid of requesting machine
 * @param url requested
 * @param new_person_guid to set
 * @param when the request started
 * @return HTTP status code. 200 on success.
 */
extern int registry_request_switch_json(struct web_client *w, char *person_guid, char *machine_guid, char *url, char *new_person_guid, time_t when);
/**
 * Public Hello request.
 *
 * Used to check if registry is responding
 *
 * @param w Requesting client.
 * @return HTTP status code. 200 on success.
 */
extern int registry_request_hello_json(struct web_client *w);

/**
 * Update the registry monitoring charts.
 */
extern void registry_statistics(void);

#endif /* NETDATA_REGISTRY_H */
