#!/usr/bin/env bash

# SPDX-License-Identifier: GPL-3.0-or-later

set -e

PLATFORMS=(
  darwin/amd64
  darwin/arm64
  freebsd/386
  freebsd/amd64
  freebsd/arm
  freebsd/arm64
  linux/386
  linux/amd64
  linux/arm
  linux/arm64
  linux/ppc64
  linux/ppc64le
  linux/mips
  linux/mipsle
  linux/mips64
  linux/mips64le
)

getos() {
  local IFS=/ && read -ra array <<<"$1" && echo "${array[0]}"
}

getarch() {
  local IFS=/ && read -ra array <<<"$1" && echo "${array[1]}"
}

usage() {
  printf 'Usage: %s [--with-traces] [all|configs|vendor|GOOS/GOARCH]\n' "${0##*/}"
}

WITH_TRACES=0
WHICH=
while (($# > 0)); do
  case "$1" in
    --with-traces)
      WITH_TRACES=1
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    --*)
      printf >&2 'Unknown option: %s\n' "$1"
      usage >&2
      exit 2
      ;;
    *)
      if [[ -n "$WHICH" ]]; then
        printf >&2 'Only one build target may be specified: %s %s\n' "$WHICH" "$1"
        usage >&2
        exit 2
      fi
      WHICH="$1"
      ;;
  esac
  shift
done

VERSION="${TRAVIS_TAG:-$(git describe --tags --always --dirty)}"

GOLDFLAGS=${GLDFLAGS:-}
GOLDFLAGS="$GOLDFLAGS -w -s -X github.com/netdata/netdata/go/plugins/pkg/buildinfo.Version=$VERSION"
if ((WITH_TRACES)); then
  GOLDFLAGS="$GOLDFLAGS -X github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/composition.jobManagerTrace=enabled"
fi

build() {
  echo "Building ${GOOS}/${GOARCH}"
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -ldflags "${GOLDFLAGS}" -o "$3" "github.com/netdata/netdata/go/plugins/cmd/godplugin"
}

create_config_archives() {
  mkdir -p bin
  tar -zcvf "bin/config.tar.gz" -C config .
  tar -zcvf "bin/go.d.plugin-config-${VERSION}.tar.gz" -C config .
}

create_vendor_archives() {
  mkdir -p bin
  go mod vendor
  tar -zc --transform "s:^:go.d.plugin-${VERSION#v}/:" -f "bin/vendor.tar.gz" vendor
  tar -zc --transform "s:^:go.d.plugin-${VERSION#v}/:" -f "bin/go.d.plugin-vendor-${VERSION}.tar.gz" vendor
}

build_all_platforms() {
  for PLATFORM in "${PLATFORMS[@]}"; do
    GOOS=$(getos "$PLATFORM")
    GOARCH=$(getarch "$PLATFORM")
    FILE="bin/go.d.plugin-${VERSION}.${GOOS}-${GOARCH}"

    build "$GOOS" "$GOARCH" "$FILE"

    ARCHIVE="${FILE}.tar.gz"
    tar -C bin -cvzf "${ARCHIVE}" "${FILE/bin\//}"
    rm "${FILE}"
  done
}

build_specific_platform() {
  GOOS=$(getos "$1")
  GOARCH=$(getarch "$1")
  : "${GOARCH:=amd64}"

  build "$GOOS" "$GOARCH" bin/godplugin
}

build_current_platform() {
  eval "$(go env | grep -e "GOHOSTOS" -e "GOHOSTARCH")"
  GOOS=${GOOS:-$GOHOSTOS}
  GOARCH=${GOARCH:-$GOHOSTARCH}

  build "$GOOS" "$GOARCH" bin/godplugin
}

if [[ "$WHICH" == "configs" ]]; then
  echo "Creating config archives for version: $VERSION"
  create_config_archives
  exit 0
fi

if [[ "$WHICH" == "vendor" ]]; then
  echo "Creating vendor archives for version: $VERSION"
  create_vendor_archives
  exit 0
fi

echo "Building binaries for version: $VERSION"

if [[ "$WHICH" == "all" ]]; then
  build_all_platforms
elif [[ -n "$WHICH" ]]; then
  build_specific_platform "$WHICH"
else
  build_current_platform
fi
