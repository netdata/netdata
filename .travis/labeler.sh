#!/bin/bash

# This is a simple script which should apply labels to unlabelled issues from last 3 days.
# It will soon be deprecated by GitHub Actions so no futher development on it is planned.

# Previously this was using POST to only add labels. But this method seems to be failing with larger number of requests
new_labels() {
	ISSUE="$1"
	URL="https://api.github.com/repos/netdata/netdata/issues/$ISSUE/labels"
	# deduplicate array and add quotes
	SET=( $(for i in "${@:2}"; do [ "$i" != "" ] && echo "\"$i\""; done | sort -u) )
	# implode array to string
	LABELS="${SET[*]}"
	# add commas between quotes (replace spaces)
	LABELS="${LABELS//\" \"/\",\"}"
	# remove duplicate quotes in case parameters were already quoted
	LABELS="${LABELS//\"\"/\"}"
	echo "-------- Assigning labels to #${ISSUE}: ${LABELS} --------"
	curl -H "Authorization: token $GITHUB_TOKEN" -d "{\"labels\":[${LABELS}]}" -X PUT "${URL}" &>/dev/null
}

if [ "$GITHUB_TOKEN" == "" ]; then
	echo "GITHUB_TOKEN is needed"
	exit 1
fi

if ! [ -x "$(command -v hub)" ]; then
	echo "===== Download HUB ====="
	HUB_VERSION=${HUB_VERSION:-"2.5.1"}
	wget "https://github.com/github/hub/releases/download/v${HUB_VERSION}/hub-linux-amd64-${HUB_VERSION}.tgz" -O "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz"
	tar -C /tmp -xvf "/tmp/hub-linux-amd64-${HUB_VERSION}.tgz" &>/dev/null
	export PATH=$PATH:"/tmp/hub-linux-amd64-${HUB_VERSION}/bin"
fi

echo "===== Looking up available labels ====="
LABELS_FILE=/tmp/labels
hub issue labels >$LABELS_FILE

echo "===== Categorizing issues ====="
# This won't touch issues which already have at least one label assigned
for STATE in "open" "closed"; do
	for ISSUE in $(hub issue -f "%I %l%n" -s "$STATE" -d "$(date +%F -d '1 day ago')" | grep -v -f $LABELS_FILE); do
		echo "-------- Processing $STATE issue no. $ISSUE --------"
		BODY="$(curl "https://api.github.com/repos/netdata/netdata/issues/$ISSUE" 2>/dev/null | jq .body)"
		case "${BODY}" in
		*"# Question summary"*) new_labels "$ISSUE" "question" "no changelog" ;;
		*"# Bug report summary"*) new_labels "$ISSUE" "needs triage" "bug" ;;
		*"# Feature idea summary"*) new_labels "$ISSUE" "needs triage" "feature request" ;;
		*) new_labels "$ISSUE" "needs triage" "no changelog" ;;
		esac
	done
done

# Change all 'area' labels assigned to PR saving non-area labels.
echo "===== Categorizing PRs ====="
NEW_LABELS=/tmp/new_labels
for PR in $(hub pr list -s all -f "%I%n" -L 10); do
	echo "----- Processing PR #$PR -----"
	echo "" >$NEW_LABELS
	NEW_SET=""
	DIFF_URL="https://github.com/netdata/netdata/pull/$PR.diff"
	for FILE in $(curl -L "${DIFF_URL}" 2>/dev/null | grep "diff --git a/" | cut -d' ' -f3 | sort | uniq); do
		LABEL=""
		case "${FILE}" in
		*".md") AREA="docs" ;;
		*"/collectors/python.d.plugin/"*) AREA="external/python" ;;
		*"/collectors/charts.d.plugin/"*) AREA="external" ;;
		*"/collectors/node.d.plugin/"*) AREA="external" ;;
		*"/.travis"*) AREA="ci" ;;
		*"/.github/*.md"*) AREA="docs" ;;
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
			echo "-------- Label '$LABEL' not available --------"
		fi
	done
	NEW_SET=$(sort $NEW_LABELS | uniq)
	if [ ! -z "$NEW_SET" ]; then
		PREV=$(curl "https://api.github.com/repos/netdata/netdata/issues/$PR/labels" 2>/dev/null | jq '.[].name' | grep -v "area")
		new_labels "$PR" ${NEW_SET} "${PREV[*]}"
		exit 0
	fi
done
