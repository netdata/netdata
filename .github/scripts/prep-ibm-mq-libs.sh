#!/bin/bash

set -e

url="$(cat packaging/vendor/ibm_mq.url)"
sha256="$(cat packaging/vendor/ibm_mq.sha256)"
file="${url##*/}"
cache_dir="artifacts/cache"
tmp_dir="tmp"
hash_path="${tmp_dir}/hash"
dl_path="${tmp_dir}/${file}"
cache_path="${cache_dir}/${file}"
extract_path="${tmp_dir}/ibm_mq"
need_to_fetch=1

mkdir -p "${tmp_dir}" "${cache_dir}" "${extract_path}"

hash_valid() {
    local path="${1}"
    echo "${sha256}  ${path}" > "${hash_path}"
    if sha256sum -c "${hash_path}"; then
        return 0
    else
        return 1
    fi
}

if [ -f "${cache_path}" ]; then
    if ! hash_valid "${cache_path}" ; then
        rm -f "${cache_path}"
    else
        need_to_fetch=0
    fi
fi

if [ "${need_to_fetch}" -eq 1 ]; then
    rm -f "${dl_path}"
    curl --output "${dl_path}" \
         --fail \
         --location \
         --max-time 300 \
         --connect-timeout 90 \
         --retry 5 \
         --retry-all-errors \
         --retry-delay 90 \
         --remote-time \
         "${url}"

    hash_valid "${dl_path}"

    cp "${dl_path}" "${cache_path}"
fi

tar -xvzf "${cache_path}" -C "${extract_path}"
