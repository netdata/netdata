# Copyright (C) 2017 Simon Nagl

# netdata is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.

# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

MAINTAINERCLEANFILES= $(srcdir)/Makefile.in

java_jar = target/java-plugin.jar

dist_java_DATA = \
	${java_jar} \
	$(NULL)

java_src = $(shell find src/ -type f -print)

${java_jar}: pom.xml ${java_src}
	./mvnw -Dmaven.user.home=.m2 -s .mvn/wrapper/settings.xml package

clean-local:
	./mvnw -Dmaven.user.home=.m2 -s .mvn/wrapper/settings.xml clean
