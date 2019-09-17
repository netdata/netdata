# #No shebang necessary
# BASH Lib: Simple incoming webhook for slack integration.
# 
# The script expects the following parameters to be defined by the upper layer:
# SLACK_NOTIFY_WEBHOOK_URL
# SLACK_BOT_NAME
# SLACK_CHANNEL
#
# Copyright:
#
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud

post_message() {
	TYPE="$1"
	MESSAGE="$2"
	CUSTOM_CHANNEL="$3"

	case "$TYPE" in
		"PLAIN_MESSAGE")
			curl -X POST --data-urlencode "payload={\"channel\": \"${SLACK_CHANNEL}\", \"username\": \"${SLACK_BOT_NAME}\", \"text\": \"${MESSAGE}\", \"icon_emoji\": \":space_invader:\"}" ${SLACK_NOTIFY_WEBHOOK_URL}
			;;
		"TRAVIS_MESSAGE")
			EVENT_LINE="${TRAVIS_JOB_NUMBER}: Event type '${TRAVIS_EVENT_TYPE}', on '${TRAVIS_OS_NAME}'"
			if [ "$TRAVIS_EVENT_TYPE}" == "pull_request" ]; then
				EVENT_LINE="${TRAVIS_JOB_NUMBER}: Event type '${TRAVIS_EVENT_TYPE}' #${TRAVIS_PULL_REQUEST}, on '${TRAVIS_OS_NAME}' "
			fi

			if [ -n "${CUSTOM_CHANNEL}" ]; then
				echo "Sending travis message to custom channel ${CUSTOM_CHANNEL}"
				OPTIONAL_CHANNEL_INFO="\"channel\": \"${CUSTOM_CHANNEL}\","
			fi

			POST_MESSAGE="{
				${OPTIONAL_CHANNEL_INFO}
				\"text\": \"${TRAVIS_REPO_SLUG}, ${MESSAGE}\",
				\"attachments\": [{
				    \"text\": \"${TRAVIS_JOB_NUMBER}: Event type '${TRAVIS_EVENT_TYPE}', on '${TRAVIS_OS_NAME}' \",
				    \"fallback\": \"I could not determine the build\",
				    \"callback_id\": \"\",
				    \"color\": \"#3AA3E3\",
				    \"attachment_type\": \"default\",
				    \"actions\": [
					{
					    \"name\": \"${TRAVIS_BUILD_NUMBER}\",
					    \"text\": \"Build #${TRAVIS_BUILD_NUMBER}\",
					    \"type\": \"button\",
					    \"url\": \"${TRAVIS_BUILD_WEB_URL}\"
					},
					{
					    \"name\": \"${TRAVIS_JOB_NUMBER}\",
					    \"text\": \"Job #${TRAVIS_JOB_NUMBER}\",
					    \"type\": \"button\",
					    \"url\": \"${TRAVIS_JOB_WEB_URL}\"
					}]
				}]
			}"
			echo "Sending ${POST_MESSAGE}"
			curl -X POST --data-urlencode "payload=${POST_MESSAGE}" "${SLACK_NOTIFY_WEBHOOK_URL}"
			;;
		*)
			echo "Unrecognized message type \"$TYPE\" was given"
			return 1
			;;
	esac
}
