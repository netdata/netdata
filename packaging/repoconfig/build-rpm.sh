#!/bin/sh

prefix='/root/rpmbuild'

if command -v dnf > /dev/null ; then
    dnf distro-sync -y --nodocs || exit 1
    dnf install -y --nodocs --setopt=install_weak_deps=False rpm-build || exit 1
elif command -v yum > /dev/null ; then
    yum distro-sync -y || exit 1
    yum install -y rpm-build || exit 1
elif command -v zypper > /dev/null ; then
    zypper update -y || exit 1
    zypper install -y rpm-build || exit 1
    prefix="/usr/src/packages"
fi

mkdir -p "${prefix}/BUILD" "${prefix}/RPMS" "${prefix}/SRPMS" "${prefix}/SPECS" "${prefix}/SOURCES" || exit 1
cp -a /netdata/packaging/repoconfig/netdata-repo.spec "${prefix}/SPECS" || exit 1
cp -a /netdata/packaging/repoconfig/* "${prefix}/SOURCES/" || exit 1

rpmbuild -bb --rebuild "${prefix}/SPECS/netdata-repo.spec" || exit 1

[ -d /netdata/artifacts ] || mkdir -p /netdata/artifacts
find "${prefix}/RPMS/" -type f -name '*.rpm' -exec cp '{}' /netdata/artifacts \; || exit 1

chown -R --reference=/netdata /netdata/artifacts
