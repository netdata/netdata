// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

#ifdef CLAIM_WITH_API

#include "registry/registry.h"

#include <curl/curl.h>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/err.h>

// Configuration
const char *claiming_directory = "/var/lib/netdata/cloud.d";
#define PRIVATE_KEY_FILE_FMT "%s/private.pem"
#define PUBLIC_KEY_FILE_FMT "%s/public.pem"

static bool create_claiming_directory() {
    struct stat st = {0};
    if (stat(claiming_directory, &st) == -1) {
        if (mkdir(claiming_directory, 0770) != 0) {
            nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to create claiming directory: %s", claiming_directory);
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

    snprintf(private_key_file, sizeof(private_key_file), PRIVATE_KEY_FILE_FMT, claiming_directory);
    snprintf(public_key_file, sizeof(public_key_file), PUBLIC_KEY_FILE_FMT, claiming_directory);

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
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to write private key: %s", private_key_file);
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

static size_t write_callback(void *ptr, size_t size, size_t nmemb, void *stream) {
    fwrite(ptr, size, nmemb, (FILE *)stream);
    return size * nmemb;
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
    {
        const char *start = rooms, *end = NULL;

        // Skip initial separators or spaces
        while (*start == ',' || *start == ' ') start++;

        // Process each item in the comma-separated list
        while ((end = strchr(start, ',')) != NULL)
            start = curl_add_json_room(wb, start, end);

        // Process the last item if any
        if (*start)
            curl_add_json_room(wb, start, &start[strlen(start)]);
    }
    buffer_json_array_close(wb);
}

static bool send_curl_request(const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, int insecure) {
    CURL *curl;
    CURLcode res;
    char target_url[256];
    char public_key_file[256];
    char public_key[2048] = "";  // Adjust size as needed
    FILE *fp;
    struct curl_slist *headers = NULL;

    snprintf(target_url, sizeof(target_url), "%s/%s", url, machine_guid);
    snprintf(public_key_file, sizeof(public_key_file), PUBLIC_KEY_FILE_FMT, claiming_directory);

    // Read the public key
    fp = fopen(public_key_file, "r");
    if (!fp || fread(public_key, 1, sizeof(public_key), fp) <= 0) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to read public key: %s", public_key_file);
        if (fp) fclose(fp);
        return false;
    }
    fclose(fp);

    CLEAN_BUFFER *wb = buffer_create(0, NULL);
    buffer_json_initialize(wb, "\"", "\"", 0, true, BUFFER_JSON_OPTIONS_MINIFY);

    buffer_json_member_add_object(wb, "node");
    {
        nd_uuid_t claimed_id;
        uuid_generate_random(claimed_id);
        char claimed_id_str[UUID_STR_LEN];
        uuid_unparse_lower(claimed_id, claimed_id_str);

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
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to initialize curl");
        return false;
    }

    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_URL, target_url);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, buffer_tostring(wb));
    curl_easy_setopt(curl, CURLOPT_HTTPHEADER, headers);
    curl_easy_setopt(curl, CURLOPT_WRITEFUNCTION, write_callback);
    curl_easy_setopt(curl, CURLOPT_WRITEDATA, stdout);

    // Proxy configuration
    if (proxy && *proxy)
        curl_easy_setopt(curl, CURLOPT_PROXY, proxy);

    // Insecure option
    if (insecure) {
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYPEER, 0L);
        curl_easy_setopt(curl, CURLOPT_SSL_VERIFYHOST, 0L);
    }

    res = curl_easy_perform(curl);
    if (res != CURLE_OK) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: curl_easy_perform() failed: %s", curl_easy_strerror(res));
        curl_easy_cleanup(curl);
        return false;
    }

    curl_easy_cleanup(curl);
    return true;
}

CLAIM_AGENT_RESPONSE claim_agent2(const char *token, const char *rooms, const char *url, const char *proxy, int insecure, const char **error) {
    if (!create_claiming_directory()) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in creating claiming directory");
        return false;
    }

    if (!check_and_generate_certificates()) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in generating or loading certificates");
        return false;
    }

    if (!send_curl_request(registry_get_this_machine_guid(), registry_get_this_machine_hostname(), token, rooms, url, proxy, insecure)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in sending curl request");
        return false;
    }

    return true;
}

CLAIM_AGENT_RESPONSE claim_agent(const char *token, const char *rooms, const char **error) {
    return claim_agent2(token, rooms, cloud_url(), cloud_proxy(), cloud_insecure(), error);
}

#endif
