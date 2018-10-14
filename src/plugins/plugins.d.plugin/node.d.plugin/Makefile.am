# SPDX-License-Identifier: GPL-3.0-or-later

MAINTAINERCLEANFILES = Makefile.in
CLEANFILES = \
	node.d.plugin \
	$(NULL)

node.d.plugin: node.d.plugin.in
	if sed \
		-e 's#[@]configdir_POST@#$(configdir)#g' \
		-e 's#[@]libconfigdir_POST@#$(libconfigdir)#g' \
		$< > $@.tmp; then \
		mv "$@.tmp" "$@"; \
	else \
		rm -f "$@.tmp"; \
		false; \
	fi

dist_plugins_SCRIPTS = \
	node.d.plugin \
	$(NULL)

dist_noinst_DATA = \
	node.d.plugin.in \
	README.md \
	named/README.md \
	fronius/README.md \
	sma_webbox/README.md \
	snmp/README.md \
	stiebeleltron/README.md \
	$(NULL)

nodeconfigdir=$(libconfigdir)/node.d
dist_nodeconfig_DATA = \
    $(NULL)

dist_node_DATA = \
	named/named.node.js \
	fronius/fronius.node.js \
	sma_webbox/sma_webbox.node.js \
	snmp/snmp.node.js \
	stiebeleltron/stiebeleltron.node.js \
	$(NULL)

nodemodulesdir=$(nodedir)/node_modules
dist_nodemodules_DATA = \
	node_modules/netdata.js \
	node_modules/extend.js \
	node_modules/pixl-xml.js \
	node_modules/net-snmp.js \
	node_modules/asn1-ber.js \
	$(NULL)

nodemoduleslibberdir=$(nodedir)/node_modules/lib/ber
dist_nodemoduleslibber_DATA = \
	node_modules/lib/ber/index.js \
	node_modules/lib/ber/errors.js \
	node_modules/lib/ber/reader.js \
	node_modules/lib/ber/types.js \
	node_modules/lib/ber/writer.js \
	$(NULL)
