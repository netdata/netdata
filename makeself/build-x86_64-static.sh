#!/usr/bin/env sh

. $(dirname "$0")/../installer/functions.sh || exit 1

set -e

DOCKER_CONTAINER_NAME="netdata-package-x86_64-static"

if ! sudo docker inspect "${DOCKER_CONTAINER_NAME}" >/dev/null 2>&1
then
    # To run interactively:
    #   sudo docker run -it netdata-package-x86_64-static /bin/sh
    # (add -v host-dir:guest-dir:rw arguments to mount volumes)
    #
    # To remove images in order to re-create:
    #   sudo docker rm -v $(sudo docker ps -a -q -f status=exited)
    #   sudo docker rmi netdata-package-x86_64-static
    #
    # This command maps the current directory to
    #   /usr/src/netdata.git
    # inside the container and runs the script install-alpine-packages.sh
    # (also inside the container)
    #
    run sudo docker run -v $(pwd):/usr/src/netdata.git:rw alpine:3.6 \
        /bin/sh /usr/src/netdata.git/makeself/install-alpine-packages.sh

    # save the changes made permanently
    id=$(sudo docker ps -l -q)
    run sudo docker commit ${id} "${DOCKER_CONTAINER_NAME}"
fi

# Run the build script inside the container
run sudo docker run -a stdin -a stdout -a stderr -i -t -v \
    $(pwd):/usr/src/netdata.git:rw \
    "${DOCKER_CONTAINER_NAME}" \
    /bin/sh /usr/src/netdata.git/makeself/build.sh "${@}"

if [ "${USER}" ]
    then
    sudo chown -R "${USER}" .
fi
