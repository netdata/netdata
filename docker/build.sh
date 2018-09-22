#!/bin/bash
# SPDX-License-Identifier: GPL-3.0+
# author  : paulfantom
# Cross-arch docker build helper script

REPOSITORY="${REPOSITORY:-netdata}"

if [ ${VERSION+x} ]; then
    VERSION="-${VERSION}"
else
    VERSION=""
fi

docker run --rm --privileged multiarch/qemu-user-static:register --reset

if [ -f Dockerfile ]; then
    cd ../ || exit 1
fi

for ARCH in i386 armhf aarch64 amd64; do
     docker build --build-arg ARCH="${ARCH}-v3.8" --tag "${REPOSITORY}:${ARCH}${VERSION}" --file docker/Dockerfile ./
done
docker tag "${REPOSITORY}:${ARCH}${VERSION}" "${REPOSITORY}:latest"

# Push images to registry
if [ -z ${DOCKER_USERNAME+x} ]; then
    echo "No docker hub username  specified. Exiting without pushing images to registry"
    exit 0
fi
if [ -z ${DOCKER_PASSWORD+x} ]; then
    echo "No docker hub password specified. Exiting without pushing images to registry"
    exit 0
fi
echo "$DOCKER_PASSWORD" | docker login -u "$DOCKER_USERNAME" --password-stdin
for ARCH in amd64 i386 armhf aarch64; do
    docker push "${REPOSITORY}:${ARCH}${VERSION}"
done
docker push "${REPOSITORY}:latest"

# TODO: Remove it after we decide to deprecate firehol/netdata docker repo
if [ "$REPOSITORY" != "netdata" ]; then
    echo "$OLD_DOCKER_PASSWORD" | docker login -u "$OLD_DOCKER_USERNAME" --password-stdin   
    for ARCH in amd64 i386 armhf aarch64; do
        docker tag ${REPOSITORY}:${ARCH}${VERSION} firehol/netdata:${ARCH}${VERSION}
        docker push "firehol/netdata:${ARCH}${VERSION}"
    done
    docker tag "${REPOSITORY}:latest"
    docker push "firehol/netdata:latest"
fi
