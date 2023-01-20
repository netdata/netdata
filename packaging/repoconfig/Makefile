FILES = netdata.list netdata-edge.list netdata-archive-keyring.gpg netdata-edge-archive-keyring.gpg netdata-repoconfig-archive-keyring.gpg

all: $(FILES)

netdata.list: netdata.list.in
	cp netdata.list.in netdata.list
	set -a && . /etc/os-release && sed -i -e "s/__DISTRO__/$${ID}/" -e "s/__SUITE__/$${VERSION_CODENAME}/" -e "s/__VARIANT__/stable/" netdata.list

netdata-edge.list: netdata.list.in
	cp netdata.list.in netdata-edge.list
	set -a && . /etc/os-release && sed -i -e "s/__DISTRO__/$${ID}/" -e "s/__SUITE__/$${VERSION_CODENAME}/" -e "s/__VARIANT__/edge/" netdata-edge.list

netdata.gpg.key:
	curl -L https://repo.netdata.cloud/netdatabot.gpg.key > $@

netdata-archive-keyring.gpg: netdata.gpg.key
	gpg --dearmor > $@ < $<

netdata-edge-archive-keyring.gpg: netdata.gpg.key
	gpg --dearmor > $@ < $<

netdata-repoconfig-archive-keyring.gpg: netdata.gpg.key
	gpg --dearmor > $@ < $<

debian/tmp:
	mkdir -p $@

install: $(FILES) debian/tmp
	cp $(FILES) debian/tmp/

clean:
	rm -f $(FILES)

.PHONY: clean
.INTERMEDIATE: netdatabot.gpg.key
