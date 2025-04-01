// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

#include "registry/registry.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

static bool check_and_generate_certificates() {
    FILE *fp;
    EVP_PKEY *pkey = NULL;
    EVP_PKEY_CTX *pctx = NULL;

    CLEAN_CHAR_P *private_key_file = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "private.pem");
    CLEAN_CHAR_P *public_key_file = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "public.pem");

    // Check if private key exists
    fp = fopen(public_key_file, "r");
    if (fp) {
        fclose(fp);
        return true;
    }

    // Generate the RSA key
    pctx = EVP_PKEY_CTX_new_id(EVP_PKEY_RSA, NULL);
    if (!pctx) {
        claim_agent_failure_reason_set("Cannot generate RSA key, EVP_PKEY_CTX_new_id() failed");
        return false;
    }

    if (EVP_PKEY_keygen_init(pctx) <= 0) {
        claim_agent_failure_reason_set("Cannot generate RSA key, EVP_PKEY_keygen_init() failed");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    if (EVP_PKEY_CTX_set_rsa_keygen_bits(pctx, 2048) <= 0) {
        claim_agent_failure_reason_set("Cannot generate RSA key, EVP_PKEY_CTX_set_rsa_keygen_bits() failed");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    if (EVP_PKEY_keygen(pctx, &pkey) <= 0) {
        claim_agent_failure_reason_set("Cannot generate RSA key, EVP_PKEY_keygen() failed");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    EVP_PKEY_CTX_free(pctx);

    // Save private key
    fp = fopen(private_key_file, "wb");
    if (!fp || !PEM_write_PrivateKey(fp, pkey, NULL, NULL, 0, NULL, NULL)) {
        claim_agent_failure_reason_set("Cannot write private key file: %s", private_key_file);
        if (fp) fclose(fp);
        EVP_PKEY_free(pkey);
        return false;
    }
    fclose(fp);

    // Save public key
    fp = fopen(public_key_file, "wb");
    if (!fp || !PEM_write_PUBKEY(fp, pkey)) {
        claim_agent_failure_reason_set("Cannot write public key file: %s", public_key_file);
        if (fp) fclose(fp);
        EVP_PKEY_free(pkey);
        return false;
    }
    fclose(fp);

    EVP_PKEY_free(pkey);
    return true;
}

static size_t response_write_callback(void *ptr, size_t size, size_t nmemb, void *stream) {
    BUFFER *wb = stream;
    size_t real_size = size * nmemb;

    buffer_memcat(wb, ptr, real_size);

    return real_size;
}

static const char *curl_add_json_room(BUFFER *wb, const char *start, const char *end, bool last_item)
{
    size_t len = end - start;

    // copy the item to an new buffer and terminate it
    char buf[len + 1];
    memcpy(buf, start, len);
    buf[len] = '\0';

    // add it to the json array
    const char *trimmed = trim(buf); // remove leading and trailing spaces
    if(trimmed)
        buffer_json_add_array_item_string(wb, trimmed);

    if (last_item)
        return NULL;

    // prepare for the next item
    start = end + 1;

    // skip multiple separators or spaces
    while(*start == ',' || *start == ' ') start++;

    return start;
}

void curl_add_rooms_json_array(BUFFER *wb, const char *rooms) {
    buffer_json_member_add_array(wb, "rooms");
    if(rooms && *rooms) {
        const char *start = rooms, *end = NULL;

        // Skip initial separators or spaces
        while (*start == ',' || *start == ' ')
            start++;

        // Process each item in the comma-separated list
        while ((end = strchr(start, ',')) != NULL)
            start = curl_add_json_room(wb, start, end, false);

        // Process the last item if any
        if (*start)
            curl_add_json_room(wb, start, &start[strlen(start)], true);
    }
    buffer_json_array_close(wb);
}

static int debug_callback(CURL *handle, curl_infotype type, char *data, size_t size, void *userptr) {
    (void)handle; // Unused
    (void)userptr; // Unused

    if (type == CURLINFO_TEXT)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Info: %s", data);
    else if (type == CURLINFO_HEADER_OUT)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Send header: %.*s", (int)size, data);
    else if (type == CURLINFO_DATA_OUT)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Send data: %.*s", (int)size, data);
    else if (type == CURLINFO_SSL_DATA_OUT)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Send SSL data: %.*s", (int)size, data);
    else if (type == CURLINFO_HEADER_IN)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Receive header: %.*s", (int)size, data);
    else if (type == CURLINFO_DATA_IN)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Receive data: %.*s", (int)size, data);
    else if (type == CURLINFO_SSL_DATA_IN)
        nd_log(NDLS_DAEMON, NDLP_INFO, "CLAIM: Receive SSL data: %.*s", (int)size, data);

    return 0;
}

static bool send_curl_request(const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, bool insecure, bool *can_retry) {
    CURL *curl;
    CURLcode res;
    char target_url[2048];
    char public_key[2048] = "";
    FILE *fp;
    struct curl_slist *headers = NULL;

    // create a new random claim id
    nd_uuid_t claimed_id;
    uuid_generate_random(claimed_id);
    char claimed_id_str[UUID_STR_LEN];
    uuid_unparse_lower(claimed_id, claimed_id_str);

    // generate the URL to post
    snprintf(target_url, sizeof(target_url), "%s%sapi/v1/spaces/nodes/%s",
             url, strendswith(url, "/") ? "" : "/", claimed_id_str);

    // Read the public key
    CLEAN_CHAR_P *public_key_file = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "public.pem");
    fp = fopen(public_key_file, "r");
    if (!fp || fread(public_key, 1, sizeof(public_key), fp) == 0) {
        claim_agent_failure_reason_set("cannot read public key file '%s'", public_key_file);
        if (fp) fclose(fp);
        *can_retry = false;
        return false;
    }
    fclose(fp);

    // check if we have trusted.pem
    // or cloud_fullchain.pem, for backwards compatibility
    CLEAN_CHAR_P *trusted_key_file = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "trusted.pem");
    fp = fopen(trusted_key_file, "r");
    if(fp)
        fclose(fp);
    else {
        freez(trusted_key_file);
        trusted_key_file = filename_from_path_entry_strdupz(netdata_configured_cloud_dir, "cloud_fullchain.pem");
        fp = fopen(trusted_key_file, "r");
        if(fp)
            fclose(fp);
        else {
            freez(trusted_key_file);
            trusted_key_file = NULL;
        }
    }

    // generate the JSON request message
    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_object(wb, "node");
    {
        buffer_json_member_add_string(wb, "id", claimed_id_str);
        buffer_json_member_add_string(wb, "hostname", hostname);
    }
    buffer_json_object_close(wb); // node

    buffer_json_member_add_string(wb, "token", token);
    curl_add_rooms_json_array(wb, rooms);
    buffer_json_member_add_string(wb, "publicKey", public_key);
    buffer_json_member_add_string(wb, "mGUID", machine_guid);
    buffer_json_finalize(wb);

    // initialize libcurl
    curl = curl_easy_init();
    if(!curl) {
        claim_agent_failure_reason_set("Cannot initialize request (curl_easy_init() failed)");
        *can_retry = true;
        return false;
    }

    // curl_easy_setopt(curl, CURLOPT_VERBOSE, 1L);
    curl_easy_setopt(curl, CURLOPT_DEBUGFUNCTION, debug_callback);

    // we will receive the response in this
    CLEAN_BUFFER *response = buffer_create(0, NULL);

    // configure the request
    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_URL, target_url);
    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, "PUT");
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, buffer_tostring(wb));
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, response_write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, response);

    if(trusted_key_file)
        curl_easy_setopt(curl, CURLOPT_CAINFO, trusted_key_file);

    // Proxy configuration
    if (proxy) {
        if (!*proxy || strcmp(proxy, "none") == 0) {
            // disable proxy configuration in libcurl
            curl_easy_setopt(curl, CURLOPT_PROXY, "");
            proxy = "none";
        }

        else if (strcmp(proxy, "env") != 0) {
            // set the custom proxy for libcurl
            curl_easy_setopt(curl, CURLOPT_PROXY, proxy);
        }

        else {
            // otherwise, libcurl will use its own proxy environment variables
            proxy = "env";
        }
    }

    // Insecure option
    if (insecure) {
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYPEER, 0L);
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYHOST, 0L);
    }

    // Set timeout options
    curl_easy_setopt(curl, CURLOPT_TIMEOUT, 10);
    curl_easy_setopt(curl, CURLOPT_CONNECTTIMEOUT, 5);

    // execute the request
    res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        claim_agent_failure_reason_set("Request failed with error: %s\n"
                                       "proxy: '%s',\n"
                                       "insecure: %s,\n"
                                       "public key file: '%s',\n"
                                       "trusted key file: '%s'",
                                       curl_easy_strerror(res),
                                       proxy,
                                       insecure ? "true" : "false",
                                       public_key_file ? public_key_file : "none",
                                       trusted_key_file ? trusted_key_file : "none");
        curl_easy_cleanup(curl);
        curl_slist_free_all(headers);
        *can_retry = true;
        return false;
    }

    // Get HTTP response code
    long http_status_code;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_status_code);

    bool ret = false;
    if(http_status_code == 204) {
        if(!cloud_conf_regenerate(claimed_id_str, machine_guid, hostname, token, rooms, url, proxy, insecure)) {
            claim_agent_failure_reason_set("Failed to save claiming info to disk");
        }
        else {
            claim_agent_failure_reason_set(NULL);
            ret = true;
        }

        *can_retry = false;
    }
    else if (http_status_code == 422) {
        if(buffer_strlen(response)) {
            struct json_object *parsed_json;
            struct json_object *error_key_obj;
            const char *error_key = NULL;

            parsed_json = json_tokener_parse(buffer_tostring(response));
            if(parsed_json) {
                if (json_object_object_get_ex(parsed_json, "errorMsgKey", &error_key_obj))
                    error_key = json_object_get_string(error_key_obj);

                if (strcmp(error_key, "ErrInvalidNodeID") == 0)
                    claim_agent_failure_reason_set("Failed: the node id is invalid");
                else if (strcmp(error_key, "ErrInvalidNodeName") == 0)
                    claim_agent_failure_reason_set("Failed: the node name is invalid");
                else if (strcmp(error_key, "ErrInvalidRoomID") == 0)
                    claim_agent_failure_reason_set("Failed: one or more room ids are invalid");
                else if (strcmp(error_key, "ErrInvalidPublicKey") == 0)
                    claim_agent_failure_reason_set("Failed: the public key is invalid");
                else
                    claim_agent_failure_reason_set("Failed with description '%s'", error_key);

                json_object_put(parsed_json);
            }
            else
                claim_agent_failure_reason_set("Failed with a response code %ld", http_status_code);
        }
        else
            claim_agent_failure_reason_set("Failed with an empty response, code %ld", http_status_code);

        *can_retry = false;
    }
    else if(http_status_code == 102) {
        claim_agent_failure_reason_set("Claiming is in progress");
        *can_retry = false;
    }
    else if(http_status_code == 403) {
        claim_agent_failure_reason_set("Failed: token is expired, not found, or invalid");
        *can_retry = false;
    }
    else if(http_status_code == 409) {
        claim_agent_failure_reason_set("Failed: agent is already claimed");
        *can_retry = false;
    }
    else if(http_status_code == 500) {
        claim_agent_failure_reason_set("Failed: received Internal Server Error");
        *can_retry = true;
    }
    else if(http_status_code == 503) {
        claim_agent_failure_reason_set("Failed: Netdata Cloud is unavailable");
        *can_retry = true;
    }
    else if(http_status_code == 504) {
        claim_agent_failure_reason_set("Failed: Gateway Timeout");
        *can_retry = true;
    }
    else {
        claim_agent_failure_reason_set("Failed with response code %ld", http_status_code);
        *can_retry = true;
    }

    curl_easy_cleanup(curl);
    curl_slist_free_all(headers);
    return ret;
}

bool claim_agent(const char *url, const char *token, const char *rooms, const char *proxy, bool insecure) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    spinlock_lock(&spinlock);

    if (!check_and_generate_certificates()) {
        spinlock_unlock(&spinlock);
        return false;
    }

    bool done = false, can_retry = true;
    size_t retries = 0;
    do {
        done = send_curl_request(machine_guid_get_txt(), registry_get_this_machine_hostname(), token, rooms, url, proxy, insecure, &can_retry);
        if (done) break;
        sleep_usec(300 * USEC_PER_MS + 100 * retries * USEC_PER_MS);
        retries++;
    } while(can_retry && retries < 5);

    spinlock_unlock(&spinlock);
    return done;
}

bool claim_agent_from_environment(void) {
    const char *url = getenv("NETDATA_CLAIM_URL");
    if(!url || !*url) {
        url = inicfg_get(&cloud_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);
        if(!url || !*url) return false;
    }

    const char *token = getenv("NETDATA_CLAIM_TOKEN");
    if(!token || !*token)
        return false;

    const char *rooms = getenv("NETDATA_CLAIM_ROOMS");
    if(!rooms)
        rooms = "";

    const char *proxy = getenv("NETDATA_CLAIM_PROXY");
    if(!proxy || !*proxy)
        proxy = "env";

    bool insecure = CONFIG_BOOLEAN_NO;
    const char *from_env = getenv("NETDATA_EXTRA_CLAIM_OPTS");
    if(from_env && *from_env && strstr(from_env, "-insecure") == 0)
        insecure = CONFIG_BOOLEAN_YES;

    return claim_agent(url, token, rooms, proxy, insecure);
}

bool claim_agent_from_claim_conf(void) {
    static struct config claim_config = APPCONFIG_INITIALIZER;
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;
    bool ret = false;

    spinlock_lock(&spinlock);

    errno_clear();
    char *filename = filename_from_path_entry_strdupz(netdata_configured_user_config_dir, "claim.conf");
    bool loaded = inicfg_load(&claim_config, filename, 1, NULL);
    freez(filename);

    if(loaded) {
        const char *url = inicfg_get(&claim_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);
        const char *token = inicfg_get(&claim_config, CONFIG_SECTION_GLOBAL, "token", "");
        const char *rooms = inicfg_get(&claim_config, CONFIG_SECTION_GLOBAL, "rooms", "");
        const char *proxy = inicfg_get(&claim_config, CONFIG_SECTION_GLOBAL, "proxy", "env");
        bool insecure = inicfg_get_boolean(&claim_config, CONFIG_SECTION_GLOBAL, "insecure", CONFIG_BOOLEAN_NO);

        if(token && *token && url && *url)
            ret = claim_agent(url, token, rooms, proxy, insecure);
    }

    spinlock_unlock(&spinlock);

    return ret;
}

bool claim_agent_from_split_files(void) {
    char filename[FILENAME_MAX + 1];

    snprintfz(filename, sizeof(filename), "%s/token", netdata_configured_cloud_dir);
    long token_len = 0;
    char *token = read_by_filename(filename, &token_len);
    if(!token || !*token) {
        freez(token);
        return false;
    }

    snprintfz(filename, sizeof(filename), "%s/rooms", netdata_configured_cloud_dir);
    long rooms_len = 0;
    char *rooms = read_by_filename(filename, &rooms_len);
    if(!rooms || !*rooms) {
        freez(rooms);
        rooms = NULL;
    }

    bool ret = claim_agent(cloud_config_url_get(), token, rooms, cloud_config_proxy_get(), cloud_config_insecure_get());

    if(ret) {
        snprintfz(filename, sizeof(filename), "%s/token", netdata_configured_cloud_dir);
        unlink(filename);

        snprintfz(filename, sizeof(filename), "%s/rooms", netdata_configured_cloud_dir);
        unlink(filename);
    }

    return ret;
}

bool claim_agent_automatically(void) {
    // Use /etc/netdata/claim.conf

    if(claim_agent_from_claim_conf())
        return true;

    // Users may set NETDATA_CLAIM_TOKEN and NETDATA_CLAIM_ROOMS
    // A good choice for docker container users.

    if(claim_agent_from_environment())
        return true;

    // Users may store token and rooms in /var/lib/netdata/cloud.d
    // This was a bad choice, since users may have to create this directory
    // which may end up with the wrong permissions, preventing netdata from storing
    // the required information there.

    if(claim_agent_from_split_files())
        return true;

    return false;
}
