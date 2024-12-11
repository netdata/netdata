// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/stm"
)

func isDockerEngineMetrics(pms prometheus.Series) bool {
	return pms.FindByName("engine_daemon_engine_info").Len() > 0
}

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if !isDockerEngineMetrics(pms) {
		return nil, fmt.Errorf("'%s' returned non docker engine metrics", c.URL)
	}

	mx := c.collectMetrics(pms)
	return stm.ToMap(mx), nil
}

func (c *Collector) collectMetrics(pms prometheus.Series) metrics {
	var mx metrics
	collectHealthChecks(&mx, pms)
	collectContainerActions(&mx, pms)
	collectBuilderBuildsFails(&mx, pms)
	if hasContainerStates(pms) {
		c.hasContainerStates = true
		mx.Container.States = &containerStates{}
		collectContainerStates(&mx, pms)
	}
	if isSwarmManager(pms) {
		c.isSwarmManager = true
		mx.SwarmManager = &swarmManager{}
		collectSwarmManager(&mx, pms)
	}
	return mx
}

func isSwarmManager(pms prometheus.Series) bool {
	return pms.FindByName("swarm_node_manager").Max() == 1
}

func hasContainerStates(pms prometheus.Series) bool {
	return pms.FindByName("engine_daemon_container_states_containers").Len() > 0
}

func collectHealthChecks(mx *metrics, raw prometheus.Series) {
	v := raw.FindByName("engine_daemon_health_checks_failed_total").Max()
	mx.HealthChecks.Failed = v
}

func collectContainerActions(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName("engine_daemon_container_actions_seconds_count") {
		action := metric.Labels.Get("action")
		if action == "" {
			continue
		}

		v := metric.Value
		switch action {
		default:
		case "changes":
			mx.Container.Actions.Changes = v
		case "commit":
			mx.Container.Actions.Commit = v
		case "create":
			mx.Container.Actions.Create = v
		case "delete":
			mx.Container.Actions.Delete = v
		case "start":
			mx.Container.Actions.Start = v
		}
	}
}

func collectContainerStates(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName("engine_daemon_container_states_containers") {
		state := metric.Labels.Get("state")
		if state == "" {
			continue
		}

		v := metric.Value
		switch state {
		default:
		case "paused":
			mx.Container.States.Paused = v
		case "running":
			mx.Container.States.Running = v
		case "stopped":
			mx.Container.States.Stopped = v
		}
	}
}

func collectBuilderBuildsFails(mx *metrics, raw prometheus.Series) {
	for _, metric := range raw.FindByName("builder_builds_failed_total") {
		reason := metric.Labels.Get("reason")
		if reason == "" {
			continue
		}

		v := metric.Value
		switch reason {
		default:
		case "build_canceled":
			mx.Builder.FailsByReason.BuildCanceled = v
		case "build_target_not_reachable_error":
			mx.Builder.FailsByReason.BuildTargetNotReachableError = v
		case "command_not_supported_error":
			mx.Builder.FailsByReason.CommandNotSupportedError = v
		case "dockerfile_empty_error":
			mx.Builder.FailsByReason.DockerfileEmptyError = v
		case "dockerfile_syntax_error":
			mx.Builder.FailsByReason.DockerfileSyntaxError = v
		case "error_processing_commands_error":
			mx.Builder.FailsByReason.ErrorProcessingCommandsError = v
		case "missing_onbuild_arguments_error":
			mx.Builder.FailsByReason.MissingOnbuildArgumentsError = v
		case "unknown_instruction_error":
			mx.Builder.FailsByReason.UnknownInstructionError = v
		}
	}
}

func collectSwarmManager(mx *metrics, raw prometheus.Series) {
	v := raw.FindByName("swarm_manager_configs_total").Max()
	mx.SwarmManager.Configs = v

	v = raw.FindByName("swarm_manager_networks_total").Max()
	mx.SwarmManager.Networks = v

	v = raw.FindByName("swarm_manager_secrets_total").Max()
	mx.SwarmManager.Secrets = v

	v = raw.FindByName("swarm_manager_services_total").Max()
	mx.SwarmManager.Services = v

	v = raw.FindByName("swarm_manager_leader").Max()
	mx.SwarmManager.IsLeader = v

	for _, metric := range raw.FindByName("swarm_manager_nodes") {
		state := metric.Labels.Get("state")
		if state == "" {
			continue
		}

		v := metric.Value
		switch state {
		default:
		case "disconnected":
			mx.SwarmManager.Nodes.PerState.Disconnected = v
		case "down":
			mx.SwarmManager.Nodes.PerState.Down = v
		case "ready":
			mx.SwarmManager.Nodes.PerState.Ready = v
		case "unknown":
			mx.SwarmManager.Nodes.PerState.Unknown = v
		}
		mx.SwarmManager.Nodes.Total += v
	}

	for _, metric := range raw.FindByName("swarm_manager_tasks_total") {
		state := metric.Labels.Get("state")
		if state == "" {
			continue
		}

		v := metric.Value
		switch state {
		default:
		case "accepted":
			mx.SwarmManager.Tasks.PerState.Accepted = v
		case "assigned":
			mx.SwarmManager.Tasks.PerState.Assigned = v
		case "complete":
			mx.SwarmManager.Tasks.PerState.Complete = v
		case "failed":
			mx.SwarmManager.Tasks.PerState.Failed = v
		case "new":
			mx.SwarmManager.Tasks.PerState.New = v
		case "orphaned":
			mx.SwarmManager.Tasks.PerState.Orphaned = v
		case "pending":
			mx.SwarmManager.Tasks.PerState.Pending = v
		case "preparing":
			mx.SwarmManager.Tasks.PerState.Preparing = v
		case "ready":
			mx.SwarmManager.Tasks.PerState.Ready = v
		case "rejected":
			mx.SwarmManager.Tasks.PerState.Rejected = v
		case "remove":
			mx.SwarmManager.Tasks.PerState.Remove = v
		case "running":
			mx.SwarmManager.Tasks.PerState.Running = v
		case "shutdown":
			mx.SwarmManager.Tasks.PerState.Shutdown = v
		case "starting":
			mx.SwarmManager.Tasks.PerState.Starting = v
		}
		mx.SwarmManager.Tasks.Total += v
	}
}
