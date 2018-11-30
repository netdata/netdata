#!/bin/bash

# This is a simple script which should apply labels to unlabelled issues from last 3 days.
# It will soon be deprecated by GitHub Actions so no futher development on it is planned.

if [ "$GITHUB_TOKEN" == "" ]; then
	echo "GITHUB_TOKEN is needed"
	exit 1
fi

# Download hub
if ! [ -x "$(command -v hub)" ]; then
	HUB_VERSION=${HUB_VERSION:-"2.5.1"}
	wget "https://github.com/github/hub/releases/download/v${HUB_VERSION}/hub-linux-amd64-${HUB_VERSION}.tgz" -O "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz"
	tar -C /tmp -xvf "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz" &>/dev/null
	export PATH=$PATH:"/tmp/hub-linux-amd64-${HUB_VERSION}/bin"
fi

echo "Looking up available labels"
LABELS_FILE=/tmp/labels
hub issue labels >$LABELS_FILE

for STATE in "open" "closed"; do
	for ISSUE in $(hub issue -f "%I %l%n" -s "$STATE" -d "$(date +%F -d '1 day ago')" | grep -v -f $LABELS_FILE); do
		echo "Processing $STATE issue no. $ISSUE"
		URL="https://api.github.com/repos/netdata/netdata/issues/$ISSUE"
		BODY="$(curl "${URL}" 2>/dev/null | jq .body)"
		case "${BODY}" in
		*"# Question summary"*) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["question"]}' -X PATCH "${URL}" ;;
		*"# Bug report summary"*) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["needs triage","bug"]}' -X PATCH "${URL}" ;;
		*"# Feature idea summary"*) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["needs triage","feature request"]}' -X PATCH "${URL}" ;;
		*) curl -H "Authorization: token $GITHUB_TOKEN" -d '{"labels":["needs triage"]}' -X PATCH "${URL}" ;;
		esac
	done
done

NEW_LABELS=/tmp/new_labels
for PR in $(hub pr list -s all -f "%I %l%n" -L 10); do
	echo "-------------------------------------------------------"
	echo "Processing PR #$PR"
	echo "" >$NEW_LABELS
	NEW_SET=""
	URL="https://api.github.com/repos/netdata/netdata/issues/$PR"
	DIFF_URL="https://github.com/netdata/netdata/pull/$PR.diff"
	for FILE in $(curl -L "${DIFF_URL}" 2>/dev/null | grep "diff --git a/" | cut -d' ' -f3 | sort | uniq); do
		LABEL=""
		case "${FILE}" in
		*".md") AREA="docs" ;;
		*"/collectors/python.d.plugin/"*) AREA="external/python" ;;
		*"/collectors/charts.d.plugin/"*) AREA="external" ;;
		*"/collectors/node.d.plugin/"*) AREA="external" ;;
		*"/.travis"*) AREA="ci" ;;
		*"/.github/"*) AREA="ci" ;;
		*"/build/"*) AREA="packaging" ;;
		*"/contrib/"*) AREA="packaging" ;;
		*"/diagrams/"*) AREA="docs" ;;
		*"/installer/"*) AREA="packaging" ;;
		*"/makeself/"*) AREA="packaging" ;;
		*"/system/"*) AREA="packaging" ;;
		*"/netdata-installer.sh"*) AREA="packaging" ;;
		*) AREA=$(echo "$FILE" | cut -d'/' -f2) ;;
		esac
		LABEL="area/$AREA"
		echo "Selecting $LABEL due to $FILE"
		if grep "$LABEL" "$LABELS_FILE"; then
			echo "$LABEL" >>$NEW_LABELS
			if [[ $LABEL =~ "external" ]]; then
				echo "area/collectors" >>$NEW_LABELS
			fi
		else
			echo "Label '$LABEL' not available"
		fi
	done
	NEW_SET=$(sort $NEW_LABELS | uniq | grep -v "^$" | sed -e 's/^/"/g;s/$/",/g' | tr -d '\n' | sed 's/.\{1\}$//')
	if [ ! -z "$NEW_SET" ]; then
		echo "Assigning labels: ${NEW_SET}"
		curl -H "Authorization: token $GITHUB_TOKEN" -d "{\"labels\":[${NEW_SET}]}" -X PATCH "${URL}" &>/dev/null
	fi
done
