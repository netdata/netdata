# SPDX-License-Identifier: GPL-3.0+
MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

dist_node_DATA = \
	README.md \
	named.node.js \
	fronius.node.js \
	sma_webbox.node.js \
	snmp.node.js \
	stiebeleltron.node.js \
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
