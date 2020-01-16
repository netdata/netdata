ARG ARCH=amd64
ARG DISTRO=debian
ARG DISTRO_VERSION=buster
ARG VERSION=0

FROM netdata/builder:${DISTRO}_${DISTRO_VERSION} AS build

# WIth no build context yet we cant use the ARG(s) above so these are dupes
ENV ARCH="${ARCH:-amd64}"
ENV DISTRO="${DISTRO:-debian}"
ENV DISTRO_VERSION="${DISTRO_VERSION:-buster}"
ENV VERSION="${VERSION:-0}"

WORKDIR /netdata
COPY . .

RUN /build.sh

FROM ${DISTRO}:${DISTRO_VERSION} AS runtime

# WIth no build context yet we cant use the ARG(s) above so these are dupes
ENV ARCH="${ARCH:-amd64}"
ENV DISTRO="${DISTRO:-debian}"
ENV DISTRO_VERSION="${DISTRO_VERSION:-buster}"
ENV VERSION="${VERSION:-0}"

# This is needed to ensure package installs don't prompt for any user input.
ENV DEBIAN_FRONTEND=noninteractive

COPY .dockerfiles/install.sh /install.sh
COPY .dockerfiles/test.sh /test.sh

COPY --from=build /netdata/artifacts /artifacts

RUN /install.sh || exit 1

CMD ["/test.sh"]
