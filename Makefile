
ifdef debug
CFLAGS=-Wall -Wextra -ggdb
else
CFLAGS=-Wall -Wextra -O3
endif

all: netdata plugins

netdata: netdata.c
	$(CC) $(CFLAGS) -o netdata netdata.c -lpthread -lz

plugins: plugins.d/apps.plugin

plugins.d/apps.plugin: apps_plugin.c
	$(CC) $(CFLAGS) -o plugins.d/apps.plugin apps_plugin.c

clean:
	rm -f *.o netdata plugins.d/apps.plugin core

install: plugins
	cp apps.plugin plugins.d/

getconf:
	mv conf.d/netdata.conf conf.d/netdata.conf.old
	wget -O conf.d/netdata.conf "http://localhost:19999/netdata.conf"
