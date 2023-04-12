#!/usr/bin/env bash
#shellcheck disable=SC2001

# netdata
# real-time performance and health monitoring, done right!
# (C) 2016 Costa Tsaousis <costa@tsaousis.gr>
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Script to find a better name for cgroups
#

export PATH="${PATH}:/sbin:/usr/sbin:/usr/local/sbin"
export LC_ALL=C

# -----------------------------------------------------------------------------

PROGRAM_NAME="$(basename "${0}")"

logdate() {
  date "+%Y-%m-%d %H:%M:%S"
}

log() {
  local status="${1}"
  shift

  echo >&2 "$(logdate): ${PROGRAM_NAME}: ${status}: ${*}"

}

warning() {
  log WARNING "${@}"
}

error() {
  log ERROR "${@}"
}

info() {
  log INFO "${@}"
}

fatal() {
  log FATAL "${@}"
  exit 1
}

function parse_docker_like_inspect_output() {
  local output="${1}"
  eval "$(grep -E "^(NOMAD_NAMESPACE|NOMAD_JOB_NAME|NOMAD_TASK_NAME|NOMAD_SHORT_ALLOC_ID|CONT_NAME|IMAGE_NAME)=" <<<"$output")"
  if [ -n "$NOMAD_NAMESPACE" ] && [ -n "$NOMAD_JOB_NAME" ] && [ -n "$NOMAD_TASK_NAME" ] && [ -n "$NOMAD_SHORT_ALLOC_ID" ]; then
    NAME="${NOMAD_NAMESPACE}-${NOMAD_JOB_NAME}-${NOMAD_TASK_NAME}-${NOMAD_SHORT_ALLOC_ID}"
  else
    NAME=$(echo "${CONT_NAME}" | sed 's|^/||')
  fi
  if [ -n "${IMAGE_NAME}" ]; then
    LABELS="image=\"${IMAGE_NAME}\""
  fi
}

function docker_like_get_name_command() {
  local command="${1}"
  local id="${2}"
  info "Running command: ${command} inspect --format='{{range .Config.Env}}{{println .}}{{end}}CONT_NAME={{ .Name}}' \"${id}\""
  if OUTPUT="$(${command} inspect --format='{{range .Config.Env}}{{println .}}{{end}}CONT_NAME={{ .Name}}{{println}}IMAGE_NAME={{ .Config.Image}}' "${id}")" &&
    [ -n "$OUTPUT" ]; then
      parse_docker_like_inspect_output "$OUTPUT"
  fi
  return 0
}

function docker_like_get_name_api() {
  local host_var="${1}"
  local host="${!host_var}"
  local path="/containers/${2}/json"
  if [ -z "${host}" ]; then
    warning "No ${host_var} is set"
    return 1
  fi
  if ! command -v jq >/dev/null 2>&1; then
    warning "Can't find jq command line tool. jq is required for netdata to retrieve container name using ${host} API, falling back to docker ps"
    return 1
  fi
  if [ -S "${host}" ]; then
    info "Running API command: curl --unix-socket \"${host}\" http://localhost${path}"
    JSON=$(curl -sS --unix-socket "${host}" "http://localhost${path}")
  else
    info "Running API command: curl \"${host}${path}\""
    JSON=$(curl -sS "${host}${path}")
  fi
  if OUTPUT=$(echo "${JSON}" | jq -r '.Config.Env[],"CONT_NAME=\(.Name)","IMAGE_NAME=\(.Config.Image)"') && [ -n "$OUTPUT" ]; then
    parse_docker_like_inspect_output "$OUTPUT"
  fi
  return 0
}

# get_lbl_val returns the value for the label with the given name.
# Returns "null" string if the label doesn't exist.
# Expected labels format: 'name="value",...'.
function get_lbl_val() {
  local labels want_name
  labels="${1}"
  want_name="${2}"

  IFS=, read -ra labels <<< "$labels"

  local lname lval
  for l in "${labels[@]}"; do
    IFS="=" read -r lname lval <<< "$l"
    if [ "$want_name" = "$lname" ] && [ -n "$lval" ]; then
      echo "${lval:1:-1}" # trim "
      return 0
    fi
  done

  echo "null"
  return 1
}

function add_lbl_prefix() {
  local orig_labels prefix
  orig_labels="${1}"
  prefix="${2}"

  IFS=, read -ra labels <<< "$orig_labels"

  local new_labels
  for l in "${labels[@]}"; do
    new_labels+="${prefix}${l},"
  done

  echo "${new_labels:0:-1}" # trim last ','
}

function k8s_is_pause_container() {
  local cgroup_path="${1}"

  local file
  if [ -d "${NETDATA_HOST_PREFIX}/sys/fs/cgroup/cpuacct" ]; then
    file="${NETDATA_HOST_PREFIX}/sys/fs/cgroup/cpuacct/$cgroup_path/cgroup.procs"
  else
    file="${NETDATA_HOST_PREFIX}/sys/fs/cgroup/$cgroup_path/cgroup.procs"
  fi

  [ ! -f "$file" ] && return 1

  local procs
  IFS= read -rd' ' procs 2>/dev/null <"$file"
  #shellcheck disable=SC2206
  procs=($procs)

  [ "${#procs[@]}" -ne 1 ] && return 1

  IFS= read -r comm 2>/dev/null <"/proc/${procs[0]}/comm"

  [ "$comm" == "pause" ]
  return
}

function k8s_gcp_get_cluster_name() {
  local header url id loc name
  header="Metadata-Flavor: Google"
  url="http://metadata/computeMetadata/v1"
  if id=$(curl --fail -s -m 3 --noproxy "*" -H "$header" "$url/project/project-id") &&
    loc=$(curl --fail -s -m 3 --noproxy "*" -H "$header" "$url/instance/attributes/cluster-location") &&
    name=$(curl --fail -s -m 3 --noproxy "*" -H "$header" "$url/instance/attributes/cluster-name") &&
    [ -n "$id" ] && [ -n "$loc" ] && [ -n "$name" ]; then
    echo "gke_${id}_${loc}_${name}"
    return 0
  fi
  return 1
}

# k8s_get_kubepod_name resolves */kubepods/* cgroup name.
# pod level cgroup name format: 'pod_<namespace>_<pod_name>'
# container level cgroup name format: 'cntr_<namespace>_<pod_name>_<container_name>'
function k8s_get_kubepod_name() {
  # GKE /sys/fs/cgroup/*/ (cri=docker, cgroups=v1):
  # |-- kubepods
  # |   |-- burstable
  # |   |   |-- pod98cee708-023b-11eb-933d-42010a800193
  # |   |   |   |-- 922161c98e6ea450bf665226cdc64ca2aa3e889934c2cff0aec4325f8f78ac03
  # |   `-- pode314bbac-d577-11ea-a171-42010a80013b
  # |       |-- 7d505356b04507de7b710016d540b2759483ed5f9136bb01a80872b08f771930
  #
  # GKE /sys/fs/cgroup/*/ (cri=containerd, cgroups=v1):
  # |-- kubepods.slice
  # |   |-- kubepods-besteffort.slice
  # |   |   |-- kubepods-besteffort-pode1465238_4518_4c21_832f_fd9f87033dad.slice
  # |   |   |   |-- cri-containerd-66be9b2efdf4d85288c319b8c1a2f50d2439b5617e36f45d9d0d0be1381113be.scope
  # |   `-- kubepods-pod91f5b561_369f_4103_8015_66391059996a.slice
  # |       |-- cri-containerd-24c53b774a586f06abc058619b47f71d9d869ac50c92898adbd199106fd0aaeb.scope
  #
  # GKE /sys/fs/cgroup/*/ (cri=crio, cgroups=v1):
  # |-- kubepods.slice
  # |   |-- kubepods-besteffort.slice
  # |   |   |-- kubepods-besteffort-podad412dfe_3589_4056_965a_592356172968.slice
  # |   |   |   |-- crio-77b019312fd9825828b70214b2c94da69c30621af2a7ee06f8beace4bc9439e5.scope
  #
  # Minikube (v1.8.2) /sys/fs/cgroup/*/ (cri=docker, cgroups=v1):
  # |-- kubepods.slice
  # |   |-- kubepods-besteffort.slice
  # |   |   |-- kubepods-besteffort-pod10fb5647_c724_400c_b9cc_0e6eae3110e7.slice
  # |   |   |   |-- docker-36e5eb5056dfdf6dbb75c0c44a1ecf23217fe2c50d606209d8130fcbb19fb5a7.scope
  #
  # kind v0.14.0
  # |-- kubelet.slice
  # |   |-- kubelet-kubepods.slice
  # |   |   |-- kubelet-kubepods-besteffort.slice
  # |   |   |   |-- kubelet-kubepods-besteffort-pod7881ed9e_c63e_4425_b5e0_ac55a08ae939.slice
  # |   |   |   |   |-- cri-containerd-00c7939458bffc416bb03451526e9fde13301d6654cfeadf5b4964a7fb5be1a9.scope
  #
  # NOTE: cgroups plugin
  # - uses '_' to join dir names (so it is <parent>_<child>_<child>_...)
  # - replaces '.' with '-'

  local fn="${FUNCNAME[0]}"
  local cgroup_path="${1}"
  local id="${2}"

  if [[ ! $id =~ ^.*kubepods.* ]]; then
    warning "${fn}: '${id}' is not kubepod cgroup."
    return 1
  fi

  local clean_id="$id"
  clean_id=${clean_id//.slice/}
  clean_id=${clean_id//.scope/}

  local name pod_uid cntr_id
  if [[ $clean_id == "kubepods" ]]; then
    name="$clean_id"
  elif [[ $clean_id =~ .+(besteffort|burstable|guaranteed)$ ]]; then
    # kubepods_<QOS_CLASS>
    # kubepods_kubepods-<QOS_CLASS>
    name=${clean_id//-/_}
    name=${name/#kubepods_kubepods/kubepods}
  elif [[ $clean_id =~ .+pod[a-f0-9_-]+_(docker|crio|cri-containerd)-([a-f0-9]+)$ ]]; then
    # ...pod<POD_UID>_(docker|crio|cri-containerd)-<CONTAINER_ID> (POD_UID w/ "_")
    cntr_id=${BASH_REMATCH[2]}
  elif [[ $clean_id =~ .+pod[a-f0-9-]+_([a-f0-9]+)$ ]]; then
    # ...pod<POD_UID>_<CONTAINER_ID>
    cntr_id=${BASH_REMATCH[1]}
  elif [[ $clean_id =~ .+pod([a-f0-9_-]+)$ ]]; then
    # ...pod<POD_UID> (POD_UID w/ and w/o "_")
    pod_uid=${BASH_REMATCH[1]}
    pod_uid=${pod_uid//_/-}
  fi

  if [ -n "$name" ]; then
    echo "$name"
    return 0
  fi

  if [ -z "$pod_uid" ] && [ -z "$cntr_id" ]; then
    warning "${fn}: can't extract pod_uid or container_id from the cgroup '$id'."
    return 3
  fi

  [ -n "$pod_uid" ] && info "${fn}: cgroup '$id' is a pod(uid:$pod_uid)"
  [ -n "$cntr_id" ] && info "${fn}: cgroup '$id' is a container(id:$cntr_id)"

  if [ -n "$cntr_id" ] && k8s_is_pause_container "$cgroup_path"; then
    return 3
  fi

  if ! command -v jq > /dev/null 2>&1; then
    warning "${fn}: 'jq' command not available."
    return 1
  fi

  local tmp_kube_cluster_name="${TMPDIR:-"/tmp"}/netdata-cgroups-k8s-cluster-name"
  local tmp_kube_system_ns_uid_file="${TMPDIR:-"/tmp"}/netdata-cgroups-kubesystem-uid"
  local tmp_kube_containers_file="${TMPDIR:-"/tmp"}/netdata-cgroups-containers"

  local kube_cluster_name
  local kube_system_uid
  local labels

  if [ -n "$cntr_id" ] &&
    [ -f "$tmp_kube_cluster_name" ] &&
    [ -f "$tmp_kube_system_ns_uid_file" ] &&
    [ -f "$tmp_kube_containers_file" ] &&
    labels=$(grep "$cntr_id" "$tmp_kube_containers_file" 2>/dev/null); then
    IFS= read -r kube_system_uid 2>/dev/null <"$tmp_kube_system_ns_uid_file"
    IFS= read -r kube_cluster_name 2>/dev/null <"$tmp_kube_cluster_name"
  else
    IFS= read -r kube_system_uid 2>/dev/null <"$tmp_kube_system_ns_uid_file"
    IFS= read -r kube_cluster_name 2>/dev/null <"$tmp_kube_cluster_name"
    [ -z "$kube_cluster_name" ] && ! kube_cluster_name=$(k8s_gcp_get_cluster_name) && kube_cluster_name="unknown"

    local kube_system_ns
    local pods

    if [ -n "${KUBERNETES_SERVICE_HOST}" ] && [ -n "${KUBERNETES_PORT_443_TCP_PORT}" ]; then
      local token header host url
      token="$(</var/run/secrets/kubernetes.io/serviceaccount/token)"
      header="Authorization: Bearer $token"
      host="$KUBERNETES_SERVICE_HOST:$KUBERNETES_PORT_443_TCP_PORT"

      if [ -z "$kube_system_uid" ]; then
        url="https://$host/api/v1/namespaces/kube-system"
        # FIX: check HTTP response code
        if ! kube_system_ns=$(curl --fail -sSk -H "$header" "$url" 2>&1); then
          warning "${fn}: error on curl '${url}': ${kube_system_ns}."
        fi
      fi

      local url
      if [ -n "${USE_KUBELET_FOR_PODS_METADATA}" ]; then
        url="${KUBELET_URL:-https://localhost:10250}/pods"
      else
        url="https://$host/api/v1/pods"
        [ -n "$MY_NODE_NAME" ] && url+="?fieldSelector=spec.nodeName==$MY_NODE_NAME"
      fi

      # FIX: check HTTP response code
      if ! pods=$(curl --fail -sSk -H "$header" "$url" 2>&1); then
        warning "${fn}: error on curl '${url}': ${pods}."
        return 1
      fi
    elif ps -C kubelet >/dev/null 2>&1 && command -v kubectl >/dev/null 2>&1; then
      if [ -z "$kube_system_uid" ]; then
        if ! kube_system_ns=$(kubectl --kubeconfig="$KUBE_CONFIG" get namespaces kube-system -o json 2>&1); then
          warning "${fn}: error on 'kubectl': ${kube_system_ns}."
        fi
      fi

      [[ -z ${KUBE_CONFIG+x} ]] && KUBE_CONFIG="/etc/kubernetes/admin.conf"
      if ! pods=$(kubectl --kubeconfig="$KUBE_CONFIG" get pods --all-namespaces -o json 2>&1); then
        warning "${fn}: error on 'kubectl': ${pods}."
        return 1
      fi
    else
      warning "${fn}: not inside the k8s cluster and 'kubectl' command not available."
      return 1
    fi

    if [ -n "$kube_system_ns" ] && ! kube_system_uid=$(jq -r '.metadata.uid' <<<"$kube_system_ns" 2>&1); then
      warning "${fn}: error on 'jq' parse kube_system_ns: ${kube_system_uid}."
    fi

    local jq_filter
    jq_filter+='.items[] | "'
    jq_filter+='namespace=\"\(.metadata.namespace)\",'
    jq_filter+='pod_name=\"\(.metadata.name)\",'
    jq_filter+='pod_uid=\"\(.metadata.uid)\",'
    #jq_filter+='\(.metadata.labels | to_entries | map("pod_label_"+.key+"=\""+.value+"\"") | join(",") | if length > 0 then .+"," else . end)'
    jq_filter+='\((.metadata.ownerReferences[]? | select(.controller==true) | "controller_kind=\""+.kind+"\",controller_name=\""+.name+"\",") // "")'
    jq_filter+='node_name=\"\(.spec.nodeName)\",'
    jq_filter+='" + '
    jq_filter+='(.status.containerStatuses[]? | "'
    jq_filter+='container_name=\"\(.name)\",'
    jq_filter+='container_id=\"\(.containerID)\"'
    jq_filter+='") | '
    jq_filter+='sub("(docker|cri-o|containerd)://";"")' # containerID: docker://a346da9bc0e3eaba6b295f64ac16e02f2190db2cef570835706a9e7a36e2c722

    local containers
    if ! containers=$(jq -r "${jq_filter}" <<<"$pods" 2>&1); then
      warning "${fn}: error on 'jq' parse pods: ${containers}."
      return 1
    fi

    [ -n "$kube_cluster_name" ] && echo "$kube_cluster_name" >"$tmp_kube_cluster_name" 2>/dev/null
    [ -n "$kube_system_ns" ] && [ -n "$kube_system_uid" ] && echo "$kube_system_uid" >"$tmp_kube_system_ns_uid_file" 2>/dev/null
    echo "$containers" >"$tmp_kube_containers_file" 2>/dev/null
  fi

  local qos_class
  if [[ $clean_id =~ .+(besteffort|burstable) ]]; then
    qos_class="${BASH_REMATCH[1]}"
  else
    qos_class="guaranteed"
  fi

  # available labels:
  # namespace, pod_name, pod_uid, container_name, container_id, node_name
  if [ -n "$cntr_id" ]; then
    if [ -n "$labels" ] || labels=$(grep "$cntr_id" <<< "$containers" 2> /dev/null); then
      labels+=',kind="container"'
      labels+=",qos_class=\"$qos_class\""
      [ -n "$kube_system_uid" ] && [ "$kube_system_uid" != "null" ] && labels+=",cluster_id=\"$kube_system_uid\""
      [ -n "$kube_cluster_name" ] && [ "$kube_cluster_name" != "unknown" ] && labels+=",cluster_name=\"$kube_cluster_name\""
      name="cntr"
      name+="_$(get_lbl_val "$labels" namespace)"
      name+="_$(get_lbl_val "$labels" pod_name)"
      name+="_$(get_lbl_val "$labels" container_name)"
      labels=$(add_lbl_prefix "$labels" "k8s_")
      name+=" $labels"
    else
      return 2
    fi
  elif [ -n "$pod_uid" ]; then
    if labels=$(grep "$pod_uid" -m 1 <<< "$containers" 2> /dev/null); then
      labels="${labels%%,container_*}"
      labels+=',kind="pod"'
      labels+=",qos_class=\"$qos_class\""
      [ -n "$kube_system_uid" ] && [ "$kube_system_uid" != "null" ] && labels+=",cluster_id=\"$kube_system_uid\""
      [ -n "$kube_cluster_name" ] && [ "$kube_cluster_name" != "unknown" ] && labels+=",cluster_name=\"$kube_cluster_name\""
      name="pod"
      name+="_$(get_lbl_val "$labels" namespace)"
      name+="_$(get_lbl_val "$labels" pod_name)"
      labels=$(add_lbl_prefix "$labels" "k8s_")
      name+=" $labels"
    else
      return 2
    fi
  fi

  # jq filter nonexistent field and nonexistent label value is 'null'
  if [[ $name =~ _null(_|$) ]]; then
    warning "${fn}: invalid name: $name (cgroup '$id')"
    if [ -n "${USE_KUBELET_FOR_PODS_METADATA}" ]; then
      # local data is cached and may not contain the correct id
      return 2
    fi
    return 1
  fi

  echo "$name"
  [ -n "$name" ]
  return
}

function k8s_get_name() {
  local fn="${FUNCNAME[0]}"
  local cgroup_path="${1}"
  local id="${2}"
  local kubepod_name=""

  kubepod_name=$(k8s_get_kubepod_name "$cgroup_path" "$id")

  case "$?" in
  0)
    kubepod_name="k8s_${kubepod_name}"

    local name labels
    name=${kubepod_name%% *}
    labels=${kubepod_name#* }

    if [ "$name" != "$labels" ]; then
      info "${fn}: cgroup '${id}' has chart name '${name}', labels '${labels}"
      NAME="$name"
      LABELS="$labels"
    else
      info "${fn}: cgroup '${id}' has chart name '${NAME}'"
      NAME="$name"
    fi
    EXIT_CODE=$EXIT_SUCCESS
    ;;
  1)
    NAME="k8s_${id}"
    warning "${fn}: cannot find the name of cgroup with id '${id}'. Setting name to ${NAME} and enabling it."
    EXIT_CODE=$EXIT_SUCCESS
    ;;
  2)
    NAME="k8s_${id}"
    warning "${fn}: cannot find the name of cgroup with id '${id}'. Setting name to ${NAME} and asking for retry."
    EXIT_CODE=$EXIT_RETRY
    ;;
  *)
    NAME="k8s_${id}"
    warning "${fn}: cannot find the name of cgroup with id '${id}'. Setting name to ${NAME} and disabling it."
    EXIT_CODE=$EXIT_DISABLE
    ;;
  esac
}

function docker_get_name() {
  local id="${1}"
  # See https://github.com/netdata/netdata/pull/13523 for details
  if command -v snap >/dev/null 2>&1 && snap list docker >/dev/null 2>&1; then
    docker_like_get_name_api DOCKER_HOST "${id}"
  elif hash docker 2> /dev/null; then
    docker_like_get_name_command docker "${id}"
  else
    docker_like_get_name_api DOCKER_HOST "${id}" || docker_like_get_name_command podman "${id}"
  fi
  if [ -z "${NAME}" ]; then
    warning "cannot find the name of docker container '${id}'"
    EXIT_CODE=$EXIT_RETRY
    NAME="${id:0:12}"
  else
    info "docker container '${id}' is named '${NAME}'"
  fi
}

function docker_validate_id() {
  local id="${1}"
  if [ -n "${id}" ] && { [ ${#id} -eq 64 ] || [ ${#id} -eq 12 ]; }; then
    docker_get_name "${id}"
  else
    error "a docker id cannot be extracted from docker cgroup '${CGROUP}'."
  fi
}

function podman_get_name() {
  local id="${1}"

  # for Podman, prefer using the API if we can, as netdata will not normally have access
  # to other users' containers, so they will not be visible when running `podman ps`
  docker_like_get_name_api PODMAN_HOST "${id}" || docker_like_get_name_command podman "${id}"

  if [ -z "${NAME}" ]; then
    warning "cannot find the name of podman container '${id}'"
    EXIT_CODE=$EXIT_RETRY
    NAME="${id:0:12}"
  else
    info "podman container '${id}' is named '${NAME}'"
  fi
}

function podman_validate_id() {
  local id="${1}"
  if [ -n "${id}" ] && [ ${#id} -eq 64 ]; then
    podman_get_name "${id}"
  else
    error "a podman id cannot be extracted from docker cgroup '${CGROUP}'."
  fi
}

# -----------------------------------------------------------------------------

DOCKER_HOST="${DOCKER_HOST:=/var/run/docker.sock}"
PODMAN_HOST="${PODMAN_HOST:=/run/podman/podman.sock}"
CGROUP_PATH="${1}" # the path as it is (e.g. '/docker/efcf4c409')
CGROUP="${2}"      # the modified path (e.g. 'docker_efcf4c409')
EXIT_SUCCESS=0
EXIT_RETRY=2
EXIT_DISABLE=3
EXIT_CODE=$EXIT_SUCCESS
NAME=
LABELS=

# -----------------------------------------------------------------------------

if [ -z "${CGROUP}" ]; then
  fatal "called without a cgroup name. Nothing to do."
fi

if [ -z "${NAME}" ]; then
  if [[ ${CGROUP} =~ ^.*kubepods.* ]]; then
    k8s_get_name "${CGROUP_PATH}" "${CGROUP}"
  fi
fi

if [ -z "${NAME}" ]; then
  if [[ ${CGROUP} =~ ^.*docker[-_/\.][a-fA-F0-9]+[-_\.]?.*$ ]]; then
    # docker containers
    #shellcheck disable=SC1117
    DOCKERID="$(echo "${CGROUP}" | sed "s|^.*docker[-_/]\([a-fA-F0-9]\+\)[-_\.]\?.*$|\1|")"
    docker_validate_id "${DOCKERID}"
  elif [[ ${CGROUP} =~ ^.*ecs[-_/\.][a-fA-F0-9]+[-_\.]?.*$ ]]; then
    # ECS
    #shellcheck disable=SC1117
    DOCKERID="$(echo "${CGROUP}" | sed "s|^.*ecs[-_/].*[-_/]\([a-fA-F0-9]\+\)[-_\.]\?.*$|\1|")"
    docker_validate_id "${DOCKERID}"
  elif [[ ${CGROUP} =~ system.slice_containerd.service_cpuset_[a-fA-F0-9]+[-_\.]?.*$ ]]; then
    # docker containers under containerd
    #shellcheck disable=SC1117
    DOCKERID="$(echo "${CGROUP}" | sed "s|^.*ystem.slice_containerd.service_cpuset_\([a-fA-F0-9]\+\)[-_\.]\?.*$|\1|")"
    docker_validate_id "${DOCKERID}"
  elif [[ ${CGROUP} =~ ^.*libpod-[a-fA-F0-9]+.*$ ]]; then
    # Podman
    PODMANID="$(echo "${CGROUP}" | sed "s|^.*libpod-\([a-fA-F0-9]\+\).*$|\1|")"
    podman_validate_id "${PODMANID}"

  elif [[ ${CGROUP} =~ machine.slice[_/].*\.service ]]; then
    # systemd-nspawn
    NAME="$(echo "${CGROUP}" | sed 's/.*machine.slice[_\/]\(.*\)\.service/\1/g')"

  elif [[ ${CGROUP} =~ machine.slice_machine.*-lxc ]]; then
    # libvirtd / lxc containers
    # machine.slice machine-lxc/x2d969/x2dhubud0xians01.scope => lxc/hubud0xians01
    # machine.slice_machine-lxc/x2d969/x2dhubud0xians01.scope/libvirt_init.scope => lxc/hubud0xians01/libvirt_init
    NAME="lxc/$(echo "${CGROUP}" | sed 's/machine.slice_machine.*-lxc//; s/[\/_]x2d[[:digit:]]*//; s/[\/_]x2d//g; s/\.scope//g')"
  elif [[ ${CGROUP} =~ machine.slice_machine.*-qemu ]]; then
    # libvirtd / qemu virtual machines
    # machine.slice_machine-qemu_x2d1_x2dopnsense.scope => qemu_opnsense
    NAME="qemu_$(echo "${CGROUP}" | sed 's/machine.slice_machine.*-qemu//; s/[\/_]x2d[[:digit:]]*//; s/[\/_]x2d//g; s/\.scope//g')"

  elif [[ ${CGROUP} =~ machine_.*\.libvirt-qemu ]]; then
    # libvirtd / qemu virtual machines
    NAME="qemu_$(echo "${CGROUP}" | sed 's/^machine_//; s/\.libvirt-qemu$//; s/-/_/;')"

  elif [[ ${CGROUP} =~ qemu.slice_([0-9]+).scope && -d /etc/pve ]]; then
    # Proxmox VMs

    FILENAME="/etc/pve/qemu-server/${BASH_REMATCH[1]}.conf"
    if [[ -f $FILENAME && -r $FILENAME ]]; then
      NAME="qemu_$(grep -e '^name: ' "/etc/pve/qemu-server/${BASH_REMATCH[1]}.conf" | head -1 | sed -rn 's|\s*name\s*:\s*(.*)?$|\1|p')"
    else
      error "proxmox config file missing ${FILENAME} or netdata does not have read access.  Please ensure netdata is a member of www-data group."
    fi
  elif [[ ${CGROUP} =~ lxc_([0-9]+) && -d /etc/pve ]]; then
    # Proxmox Containers (LXC)

    FILENAME="/etc/pve/lxc/${BASH_REMATCH[1]}.conf"
    if [[ -f ${FILENAME} && -r ${FILENAME} ]]; then
      NAME=$(grep -e '^hostname: ' "/etc/pve/lxc/${BASH_REMATCH[1]}.conf" | head -1 | sed -rn 's|\s*hostname\s*:\s*(.*)?$|\1|p')
    else
      error "proxmox config file missing ${FILENAME} or netdata does not have read access.  Please ensure netdata is a member of www-data group."
    fi
  elif [[ ${CGROUP} =~ lxc.payload.* ]]; then
    # LXC 4.0
    NAME="$(echo "${CGROUP}" | sed 's/lxc\.payload\.\(.*\)/\1/g')"
  fi

  [ -z "${NAME}" ] && NAME="${CGROUP}"
  [ ${#NAME} -gt 100 ] && NAME="${NAME:0:100}"
fi

NAME="${NAME// /_}"

info "cgroup '${CGROUP}' is called '${NAME}', labels '${LABELS}'"
if [ -n "$LABELS" ]; then
  echo "${NAME} ${LABELS}"
else
  echo "${NAME}"
fi

exit ${EXIT_CODE}
