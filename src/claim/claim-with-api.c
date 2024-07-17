// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

#include "registry/registry.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

// Configuration
#define PRIVATE_KEY_FILE_FMT "%s/private.pem"
#define PUBLIC_KEY_FILE_FMT "%s/public.pem"

static bool create_claiming_directory() {
    struct stat st = {0};
    if (stat(netdata_configured_cloud_dir, &st) == -1) {
        if (mkdir(netdata_configured_cloud_dir, 0770) != 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR,
                   "CLAIM: Failed to create claiming directory: %s",
                   netdata_configured_cloud_dir);
            return false;
        }
    }
    return true;
}

static bool check_and_generate_certificates() {
    FILE *fp;
    EVP_PKEY *pkey = NULL;
    EVP_PKEY_CTX *pctx = NULL;
    char private_key_file[256];
    char public_key_file[256];

    snprintf(private_key_file, sizeof(private_key_file), PRIVATE_KEY_FILE_FMT, netdata_configured_cloud_dir);
    snprintf(public_key_file, sizeof(public_key_file), PUBLIC_KEY_FILE_FMT, netdata_configured_cloud_dir);

    // Check if private key exists
    fp = fopen(private_key_file, "r");
    if (fp) {
        fclose(fp);
        return true; // Keys already exist
    }

    // Generate the RSA key
    pctx = EVP_PKEY_CTX_new_id(EVP_PKEY_RSA, NULL);
    if (!pctx) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: EVP_PKEY_CTX_new_id error");
        return false;
    }

    if (EVP_PKEY_keygen_init(pctx) <= 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: EVP_PKEY_keygen_init error");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    if (EVP_PKEY_CTX_set_rsa_keygen_bits(pctx, 2048) <= 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: EVP_PKEY_CTX_set_rsa_keygen_bits error");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    if (EVP_PKEY_keygen(pctx, &pkey) <= 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: EVP_PKEY_keygen error");
        EVP_PKEY_CTX_free(pctx);
        return false;
    }

    EVP_PKEY_CTX_free(pctx);

    // Save private key
    fp = fopen(private_key_file, "wb");
    if (!fp || !PEM_write_PrivateKey(fp, pkey, NULL, NULL, 0, NULL, NULL)) {
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: Failed to write private key: %s", private_key_file);
        if (fp) fclose(fp);
        EVP_PKEY_free(pkey);
        return false;
    }
    fclose(fp);

    // Save public key
    fp = fopen(public_key_file, "wb");
    if (!fp || !PEM_write_PUBKEY(fp, pkey)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to write public key: %s", public_key_file);
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

static const char *curl_add_json_room(BUFFER *wb, const char *start, const char *end) {
    size_t len = end - start;

    // copy the item to an new buffer and terminate it
    char buf[len + 1];
    memcpy(buf, start, len);
    buf[len] = '\0';

    // add it to the json array
    const char *trimmed = trim(buf); // remove leading and trailing spaces
    if(trimmed)
        buffer_json_add_array_item_string(wb, trimmed);

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
            start = curl_add_json_room(wb, start, end);

        // Process the last item if any
        if (*start)
            curl_add_json_room(wb, start, &start[strlen(start)]);
    }
    buffer_json_array_close(wb);
}

static bool send_curl_request(const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, int insecure, const char **error) {
    CURL *curl;
    CURLcode res;
    char target_url[1024];
    char public_key_file[256];
    char public_key[2048] = "";  // Adjust size as needed
    FILE *fp;
    struct curl_slist *headers = NULL;

    // create a new random claim id
    nd_uuid_t claimed_id;
    uuid_generate_random(claimed_id);
    char claimed_id_str[UUID_STR_LEN];
    uuid_unparse_lower(claimed_id, claimed_id_str);

    // generate the URL to post
    snprintf(target_url, sizeof(target_url), "%s/api/v1/spaces/nodes/%s", url, claimed_id_str);

    // Read the public key
    snprintf(public_key_file, sizeof(public_key_file), PUBLIC_KEY_FILE_FMT, netdata_configured_cloud_dir);
    fp = fopen(public_key_file, "r");
    if (!fp || fread(public_key, 1, sizeof(public_key), fp) <= 0) {
        *error = "Failed to read public key";
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: %s: %s", *error, public_key_file);
        if (fp) fclose(fp);
        return false;
    }
    fclose(fp);

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

    curl = curl_easy_init();
    if(!curl) {
        *error = "Failed to initialize libcurl";
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: %s", *error);
        return false;
    }

    CLEAN_BUFFER *response = buffer_create(0, NULL);

    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_URL, target_url);
    curl_easy_setopt(curl, CURLOPT_CUSTOMREQUEST, "PUT");
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, buffer_tostring(wb));
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, response_write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, response);

    // Proxy configuration
    if (proxy && *proxy && strcmp(proxy, "none") != 0 && strcmp(proxy, "env") != 0)
        curl_easy_setopt(curl, CURLOPT_PROXY, proxy);

    // Insecure option
    if (insecure) {
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYPEER, 0L);
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYHOST, 0L);
    }

    res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        *error = "Failed to make HTTPS request";
        nd_log(NDLS_DAEMON, NDLP_ERR,
               "CLAIM: %s: %s", *error, curl_easy_strerror(res));
        curl_easy_cleanup(curl);
        return false;
    }

    // Get HTTP response code
    long http_status_code;
    curl_easy_getinfo(curl, CURLINFO_RESPONSE_CODE, &http_status_code);

    bool ret = false;
    if(http_status_code == 204) {
        *error = "Agent claimed successfully";
        ret = true;
        cloud_conf_regenerate(claimed_id_str, machine_guid, hostname, token, rooms, url, proxy, insecure);
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
                    *error = "Invalid node id";
                else if (strcmp(error_key, "ErrInvalidNodeName") == 0)
                    *error = "Invalid node name";
                else if (strcmp(error_key, "ErrInvalidRoomID") == 0)
                    *error = "Invalid room id";
                else if (strcmp(error_key, "ErrInvalidPublicKey") == 0)
                    *error = "Invalid public key";
                else
                    *error = "Failed with unknown error reason in response";

                json_object_put(parsed_json);
            }
            else
                *error = "Failed to parse JSON response";
        }
        else
            *error = "Failed with empty JSON response";
    }
    else if(http_status_code == 102)
        *error = "Processing claiming";
    else if(http_status_code == 403)
        *error = "Token expired/token not found/invalid token";
    else if(http_status_code == 409)
        *error = "Already claimed";
    else if(http_status_code == 500)
        *error = "Internal server error";
    else if(http_status_code == 503)
        *error = "Service unavailable";
    else if(http_status_code == 504)
        *error = "Gateway timeout";
    else
        *error = "Unknown HTTP response code";

    curl_easy_cleanup(curl);
    return ret;
}

bool claim_agent_with_checks(const char *token, const char *rooms, const char *url, const char *proxy, int insecure, const char **error) {
    *error = "OK";

    if (!create_claiming_directory()) {
        *error = "Failed to create claim directory";
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: %s", *error);
        return false;
    }

    if (!check_and_generate_certificates()) {
        *error = "Failed to generate certificates";
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: %s", *error);
        return false;
    }

    if (!send_curl_request(registry_get_this_machine_guid(), registry_get_this_machine_hostname(), token, rooms, url, proxy, insecure, error)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: %s", *error);
        return false;
    }

    return true;
}

bool claim_agent(const char *url, const char *token, const char *rooms, const char *proxy, bool insecure) {
    const char *msg = NULL;
    bool rc = claim_agent_with_checks(token, rooms, url, proxy, insecure, &msg);

    if(rc)
        claim_agent_failure_reason_set(NULL);
    else
        claim_agent_failure_reason_set(msg && *msg ? msg : "Unknown error");

    appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "url", url);
    appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "token", token ? token : "");
    appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "rooms", rooms ? rooms : "");
    appconfig_set(&cloud_config, CONFIG_SECTION_GLOBAL, "proxy", proxy ? proxy : "");
    appconfig_set_boolean(&cloud_config, CONFIG_SECTION_GLOBAL, "insecure", insecure);

    return rc;
}

bool claim_agent_from_environment(void) {
    const char *url = getenv("NETDATA_CLAIM_URL");
    if(!url || !*url) {
        url = appconfig_get(&cloud_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);
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
        proxy = "";

    bool insecure = CONFIG_BOOLEAN_NO;
    const char *from_env = getenv("NETDATA_EXTRA_CLAIM_OPTS");
    if(from_env && *from_env && strstr(from_env, "-insecure") == 0)
        insecure = CONFIG_BOOLEAN_YES;

    return claim_agent(url, token, rooms, proxy, insecure);
}

bool claim_agent_from_claim_conf(void) {
    static struct config claim_config = {
        .first_section = NULL,
        .last_section = NULL,
        .mutex = NETDATA_MUTEX_INITIALIZER,
        .index = {
            .avl_tree = {
                .root = NULL,
                .compar = appconfig_section_compare
            },
            .rwlock = AVL_LOCK_INITIALIZER
        }
    };
    static SPINLOCK spinlock = NETDATA_SPINLOCK_INITIALIZER;
    bool ret = false;

    spinlock_lock(&spinlock);

    errno_clear();
    char *filename = strdupz_path_subpath(netdata_configured_user_config_dir, "claim.conf");
    bool loaded = appconfig_load(&claim_config, filename, 1, NULL);
    freez(filename);

    if(loaded) {
        const char *url = appconfig_get(&claim_config, CONFIG_SECTION_GLOBAL, "url", DEFAULT_CLOUD_BASE_URL);
        const char *token = appconfig_get(&claim_config, CONFIG_SECTION_GLOBAL, "token", "");
        const char *rooms = appconfig_get(&claim_config, CONFIG_SECTION_GLOBAL, "rooms", "");
        const char *proxy = appconfig_get(&claim_config, CONFIG_SECTION_GLOBAL, "proxy", "");
        bool insecure = appconfig_get_boolean(&claim_config, CONFIG_SECTION_GLOBAL, "insecure", CONFIG_BOOLEAN_NO);

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
    if(!token || !*token)
        return false;

    snprintfz(filename, sizeof(filename), "%s/rooms", netdata_configured_cloud_dir);
    long rooms_len = 0;
    char *rooms = read_by_filename(filename, &rooms_len);
    if(!rooms || !*rooms)
        rooms = NULL;

    bool ret = claim_agent(cloud_url(), token, rooms, cloud_proxy(), cloud_insecure());

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
