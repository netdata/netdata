#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0+

path=$(dirname "$0")
cd "${path}" || exit 1

if [ ! -f "plantuml.jar" ]
then
	echo >&2 "Please download 'plantuml.jar' from http://plantuml.com/ and put it the same folder with me."
	exit 1
fi

for x in *.puml
do
	[ "${x}" = "config.puml" ] && continue

	echo >&2 "Working on ${x}..."
	java -jar plantuml.jar -tpng "${x}"
	java -jar plantuml.jar -tsvg "${x}"
	# java -jar plantuml.jar -ttxt "${x}"
done
