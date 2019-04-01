# #No shebang necessary
# BASH Lib: Simple incoming webhook for slack integration.
# 
# The script expects the following parameters to be defined by the upper layer:
# SLACK_INCOMING_WEBHOOK_URL
# SLACK_BOT_NAME
# SLACK_CHANNEL
#
# Copyright:
#
# Author: Pavlos Emm. Katsoulakis <paul@netdata.cloud

post_message() {
	MESSAGE="$1"
	curl -X POST --data-urlencode "payload={\"channel\": \"${SLACK_CHANNEL}\", \"username\": \"${SLACK_BOT_NAME}\", \"text\": \"${MESSAGE}\", \"icon_emoji\": \":space_invader:\"}" ${SLACK_INCOMING_WEBHOOK_URL}
}
