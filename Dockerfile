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

COPY .dockerfiles/test.sh /test.sh

COPY --from=build /netdata_${VERSION}_${ARCH}.deb .

RUN apt-get update && \
	apt-get install -y curl netcat jq && \
	apt install -y /netdata_${VERSION}_${ARCH}.deb

CMD ["/test.sh"]
