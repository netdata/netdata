FILES = netdata.list netdata-edge.list netdata-archive-keyring.gpg netdata-edge-archive-keyring.gpg netdata-repoconfig-archive-keyring.gpg

all: $(FILES)

netdata.list: netdata.list.in
	cp netdata.list.in netdata.list
	set -a && . /etc/os-release && sed -i -e "s/__DISTRO__/$${ID}/" -e "s/__SUITE__/$${VERSION_CODENAME}/" -e "s/__VARIANT__//" netdata.list

netdata-edge.list: netdata.list.in
	cp netdata.list.in netdata-edge.list
	set -a && . /etc/os-release && sed -i -e "s/__DISTRO__/$${ID}/" -e "s/__SUITE__/$${VERSION_CODENAME}/" -e "s/__VARIANT__/-edge/" netdata-edge.list

netdata-archive-keyring.gpg:
	curl -L https://packagecloud.io/netdata/netdata/gpgkey | gpg --dearmor > netdata-archive-keyring.gpg

netdata-edge-archive-keyring.gpg:
	curl -L https://packagecloud.io/netdata/netdata-edge/gpgkey | gpg --dearmor > netdata-edge-archive-keyring.gpg

netdata-repoconfig-archive-keyring.gpg:
	curl -L https://packagecloud.io/netdata/netdata-repoconfig/gpgkey | gpg --dearmor > netdata-repoconfig-archive-keyring.gpg

debian/tmp:
	mkdir -p debian/tmp

install: $(FILES) debian/tmp
	cp $(FILES) debian/tmp/

clean:
	rm -f $(FILES)

.PHONY: clean
