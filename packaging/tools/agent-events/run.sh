#!/usr/bin/env bash

stdbuf -oL /opt/agent-events/server --port=30001 --dedup-key agent.ephemeral_id --dedup-window 1800 2>/dev/null \
	| stdbuf -oL log2journal json \
		--prefix 'AE_' \
		--inject 'SYSLOG_IDENTIFIER=agent-events' \
		--rename AE_AGENT_PROFILE_0=AE_AGENT_ND_PROFILE_0 \
		--rename AE_AGENT_PROFILE_1=AE_AGENT_ND_PROFILE_1 \
		--rename AE_AGENT_PROFILE_2=AE_AGENT_ND_PROFILE_2 \
		--rename AE_STATUS=AE_AGENT_STATUS \
		--rename AE_STATUS=AE_AGENT_ND_STATUS \
		--rename AE_AGENT_EXIT_REASON_0=AE_AGENT_ND_EXIT_REASON_0 \
		--rename AE_AGENT_EXIT_REASON_1=AE_AGENT_ND_EXIT_REASON_1 \
		--rename AE_AGENT_EXIT_REASON_2=AE_AGENT_ND_EXIT_REASON_2 \
		--rename AE_AGENT_EXIT_REASON_3=AE_AGENT_ND_EXIT_REASON_3 \
		--rename AE_AGENT_NODE_ID=AE_AGENT_ND_NODE_ID \
		--rename AE_AGENT_CLAIM_ID=AE_AGENT_ND_CLAIM_ID \
		--rename AE_AGENT_INSTALL_TYPE=AE_AGENT_ND_INSTALL_TYPE \
		--rename AE_AGENT_RESTARTS=AE_AGENT_ND_RESTARTS \
		--rename AE_AGENT_DB_MODE=AE_AGENT_ND_DB_MODE \
		--rename AE_AGENT_DB_TIERS=AE_AGENT_ND_DB_TIERS \
		--rename AE_AGENT_KUBERNETES=AE_AGENT_ND_KUBERNETES \
		--rename AE_AGENT_SENTRY_AVAILABLE=AE_AGENT_ND_SENTRY_AVAILABLE \
		--rename AE_AGENT_SENTRY_AVAILABLE=AE_AGENT_ND_SENTRY \
		--rename PRIORITY=AE_PRIORITY \
		--rename MESSAGE=AE_MESSAGE \
		--rewrite 'AE_FATAL_LINE=/0//' \
		--rewrite 'AE_FATAL_THREAD_ID=/0//' \
		--rewrite 'AE_OS_PLATFORM=/unknown/${AE_OS_FAMILY}/' \
		--rewrite 'MESSAGE=/.*/${MESSAGE}\n${AE_FATAL_MESSAGE}/' \
	| systemd-cat-native --newline=\\n
