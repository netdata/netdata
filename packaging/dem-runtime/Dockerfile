# SPDX-License-Identifier: GPL-3.0-or-later
ARG BASE=debian@sha256:5bc3287b25407c965a30f38e32603dc253a3869e1b12a21ac09bfc27fd8b13ce
FROM ${BASE}
COPY debs/ /opt/dem-debs/
# Every archive was hash-verified by build.py. There are no remote apt indexes,
# and BuildKit denies networking during this local-only dependency transaction.
RUN --network=none set -eu; \
    mkdir /opt/dem-empty-sources; touch /opt/dem-empty-sources.list; rm -rf /var/lib/apt/lists/*; \
    printf '%s\n' '+ apt-get with empty sources: install locked local archives' >&2; \
    DEBIAN_FRONTEND=noninteractive apt-get -o Dir::Etc::sourcelist=/opt/dem-empty-sources.list -o Dir::Etc::sourceparts=/opt/dem-empty-sources --no-install-recommends -y install /opt/dem-debs/*.deb; \
    rm -rf /opt/dem-debs /var/lib/apt/lists/*
