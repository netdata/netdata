#!/usr/bin/env bash
me="$(basename "${0}")"

# Checks if netdata is running in a kubernetes pod and fetches:
#   - pod's labels
#   - kubernetes cluster name (GKE only)

if [ -z "${KUBERNETES_SERVICE_HOST}" ] || [ -z "${KUBERNETES_PORT_443_TCP_PORT}" ] || [ -z "${MY_POD_NAMESPACE}" ] || [ -z "${MY_POD_NAME}" ]; then
  exit 0
fi

if ! command -v jq >/dev/null 2>&1; then
  echo >&2 "${me}: jq command not available. Please install jq to get host labels for kubernetes pods."
  exit 1
fi

TOKEN="$(< /var/run/secrets/kubernetes.io/serviceaccount/token)"
HEADER="Authorization: Bearer $TOKEN"
HOST="$KUBERNETES_SERVICE_HOST:$KUBERNETES_PORT_443_TCP_PORT"

URL="https://$HOST/api/v1/namespaces/$MY_POD_NAMESPACE/pods/$MY_POD_NAME"
if ! POD_DATA=$(curl --fail -sSk -H "$HEADER" "$URL" 2>&1); then
  echo >&2 "${me}: error on curl '${URL}': ${POD_DATA}."
  exit 1
fi

URL="https://$HOST/api/v1/namespaces/kube-system"
if ! KUBE_SYSTEM_NS_DATA=$(curl --fail -sSk -H "$HEADER" "$URL" 2>&1); then
  echo >&2 "${me}: error on curl '${URL}': ${KUBE_SYSTEM_NS_DATA}."
  exit 1
fi

if ! POD_LABELS=$(jq -r '.metadata.labels' <<< "$POD_DATA" | grep ':' | tr -d '," ' 2>&1); then
  echo >&2 "${me}: error on 'jq' parse pod data: ${POD_LABELS}."
  exit 1
fi

if ! KUBE_SYSTEM_NS_UID=$(jq -r '.metadata.uid' <<< "$KUBE_SYSTEM_NS_DATA" 2>&1); then
  echo >&2 "${me}: error on 'jq' parse kube_system_ns: ${KUBE_SYSTEM_NS_UID}."
  exit 1
fi

LABELS="$POD_LABELS\nk8s_cluster_id:$KUBE_SYSTEM_NS_UID"

GCP_META_HEADER="Metadata-Flavor: Google"
GCP_META_URL="http://metadata/computeMetadata/v1"
GKE_CLUSTER_NAME=""

if id=$(curl --fail -s -m 5 --noproxy "*" -H "$GCP_META_HEADER" "$GCP_META_URL/project/project-id"); then
  loc=$(curl --fail -s -m 5 --noproxy "*" -H "$GCP_META_HEADER" "$GCP_META_URL/instance/attributes/cluster-location")
  name=$(curl --fail -s -m 5 --noproxy "*" -H "$GCP_META_HEADER" "$GCP_META_URL/instance/attributes/cluster-name")
  [ -n "$id" ] && [ -n "$loc" ] && [ -n "$name" ] && GKE_CLUSTER_NAME="gke_${id}_${loc}_${name}"
fi

[ -n "$GKE_CLUSTER_NAME" ] && LABELS+="\nk8s_cluster_name:$GKE_CLUSTER_NAME"

echo -e "$LABELS"

exit 0
