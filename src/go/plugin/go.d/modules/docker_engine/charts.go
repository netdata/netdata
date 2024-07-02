// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	Charts = module.Charts
	Dims   = module.Dims
)

var charts = Charts{
	{
		ID:    "engine_daemon_container_actions",
		Title: "Container Actions",
		Units: "actions/s",
		Fam:   "containers",
		Ctx:   "docker_engine.engine_daemon_container_actions",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "container_actions_changes", Name: "changes", Algo: module.Incremental},
			{ID: "container_actions_commit", Name: "commit", Algo: module.Incremental},
			{ID: "container_actions_create", Name: "create", Algo: module.Incremental},
			{ID: "container_actions_delete", Name: "delete", Algo: module.Incremental},
			{ID: "container_actions_start", Name: "start", Algo: module.Incremental},
		},
	},
	{
		ID:    "engine_daemon_container_states_containers",
		Title: "Containers In Various States",
		Units: "containers",
		Fam:   "containers",
		Ctx:   "docker_engine.engine_daemon_container_states_containers",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "container_states_running", Name: "running"},
			{ID: "container_states_paused", Name: "paused"},
			{ID: "container_states_stopped", Name: "stopped"},
		},
	},
	{
		ID:    "builder_builds_failed_total",
		Title: "Builder Builds Fails By Reason",
		Units: "fails/s",
		Fam:   "builder",
		Ctx:   "docker_engine.builder_builds_failed_total",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "builder_fails_build_canceled", Name: "build_canceled", Algo: module.Incremental},
			{ID: "builder_fails_build_target_not_reachable_error", Name: "build_target_not_reachable_error", Algo: module.Incremental},
			{ID: "builder_fails_command_not_supported_error", Name: "command_not_supported_error", Algo: module.Incremental},
			{ID: "builder_fails_dockerfile_empty_error", Name: "dockerfile_empty_error", Algo: module.Incremental},
			{ID: "builder_fails_dockerfile_syntax_error", Name: "dockerfile_syntax_error", Algo: module.Incremental},
			{ID: "builder_fails_error_processing_commands_error", Name: "error_processing_commands_error", Algo: module.Incremental},
			{ID: "builder_fails_missing_onbuild_arguments_error", Name: "missing_onbuild_arguments_error", Algo: module.Incremental},
			{ID: "builder_fails_unknown_instruction_error", Name: "unknown_instruction_error", Algo: module.Incremental},
		},
	},
	{
		ID:    "engine_daemon_health_checks_failed_total",
		Title: "Health Checks",
		Units: "events/s",
		Fam:   "health checks",
		Ctx:   "docker_engine.engine_daemon_health_checks_failed_total",
		Dims: Dims{
			{ID: "health_checks_failed", Name: "fails", Algo: module.Incremental},
		},
	},
}

var swarmManagerCharts = Charts{
	{
		ID:    "swarm_manager_leader",
		Title: "Swarm Manager Leader",
		Units: "bool",
		Fam:   "swarm",
		Ctx:   "docker_engine.swarm_manager_leader",
		Dims: Dims{
			{ID: "swarm_manager_leader", Name: "is_leader"},
		},
	},
	{
		ID:    "swarm_manager_object_store",
		Title: "Swarm Manager Object Store",
		Units: "objects",
		Fam:   "swarm",
		Type:  module.Stacked,
		Ctx:   "docker_engine.swarm_manager_object_store",
		Dims: Dims{
			{ID: "swarm_manager_nodes_total", Name: "nodes"},
			{ID: "swarm_manager_services_total", Name: "services"},
			{ID: "swarm_manager_tasks_total", Name: "tasks"},
			{ID: "swarm_manager_networks_total", Name: "networks"},
			{ID: "swarm_manager_secrets_total", Name: "secrets"},
			{ID: "swarm_manager_configs_total", Name: "configs"},
		},
	},
	{
		ID:    "swarm_manager_nodes_per_state",
		Title: "Swarm Manager Nodes Per State",
		Units: "nodes",
		Fam:   "swarm",
		Ctx:   "docker_engine.swarm_manager_nodes_per_state",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "swarm_manager_nodes_state_ready", Name: "ready"},
			{ID: "swarm_manager_nodes_state_down", Name: "down"},
			{ID: "swarm_manager_nodes_state_unknown", Name: "unknown"},
			{ID: "swarm_manager_nodes_state_disconnected", Name: "disconnected"},
		},
	},
	{
		ID:    "swarm_manager_tasks_per_state",
		Title: "Swarm Manager Tasks Per State",
		Units: "tasks",
		Fam:   "swarm",
		Ctx:   "docker_engine.swarm_manager_tasks_per_state",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "swarm_manager_tasks_state_running", Name: "running"},
			{ID: "swarm_manager_tasks_state_failed", Name: "failed"},
			{ID: "swarm_manager_tasks_state_ready", Name: "ready"},
			{ID: "swarm_manager_tasks_state_rejected", Name: "rejected"},
			{ID: "swarm_manager_tasks_state_starting", Name: "starting"},
			{ID: "swarm_manager_tasks_state_shutdown", Name: "shutdown"},
			{ID: "swarm_manager_tasks_state_new", Name: "new"},
			{ID: "swarm_manager_tasks_state_orphaned", Name: "orphaned"},
			{ID: "swarm_manager_tasks_state_preparing", Name: "preparing"},
			{ID: "swarm_manager_tasks_state_pending", Name: "pending"},
			{ID: "swarm_manager_tasks_state_complete", Name: "complete"},
			{ID: "swarm_manager_tasks_state_remove", Name: "remove"},
			{ID: "swarm_manager_tasks_state_accepted", Name: "accepted"},
			{ID: "swarm_manager_tasks_state_assigned", Name: "assigned"},
		},
	},
}
