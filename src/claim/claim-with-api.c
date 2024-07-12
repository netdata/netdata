// SPDX-License-Identifier: GPL-3.0-or-later

#include "claim.h"

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

    char json_payload[4096];
    snprintf(json_payload, sizeof(json_payload),
             "{\"node\": {\"id\": \"%s\", \"hostname\": \"%s\"}, \"token\": \"%s\", \"rooms\": [ \"%s\" ], \"publicKey\": \"%s\", \"mGUID\": \"%s\"}",
             machine_guid, hostname, token, rooms, public_key, machine_guid);

    curl_global_init(CURL_GLOBAL_ALL);
    curl = curl_easy_init();
    if(!curl) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Failed to initialize curl");
        return false;
    }

    headers = curl_slist_append(headers, "Content-Type: application/json");
    curl_easy_setopt(curl, CURLOPT_URL, target_url);
    curl_easy_setopt(curl, CURLOPT_POSTFIELDS, json_payload);
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
    curl_global_cleanup();
    return true;
}

bool claim_agent2(const char *machine_guid, const char *hostname, const char *token, const char *rooms, const char *url, const char *proxy, int insecure) {
    if (!create_claiming_directory()) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in creating claiming directory");
        return false;
    }
    if (!check_and_generate_certificates()) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in generating or loading certificates");
        return false;
    }
    if (!send_curl_request(machine_guid, hostname, token, rooms, url, proxy, insecure)) {
        nd_log(NDLS_DAEMON, NDLP_ERR, "CLAIM: Error in sending curl request");
        return false;
    }
    return true;
}
