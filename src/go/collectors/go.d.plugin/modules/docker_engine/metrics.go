// SPDX-License-Identifier: GPL-3.0-or-later

package docker_engine

type metrics struct {
	Container struct {
		Actions struct {
			Changes float64 `stm:"changes"`
			Commit  float64 `stm:"commit"`
			Create  float64 `stm:"create"`
			Delete  float64 `stm:"delete"`
			Start   float64 `stm:"start"`
		} `stm:"actions"`
		States *containerStates `stm:"states"`
	} `stm:"container"`
	Builder struct {
		FailsByReason struct {
			BuildCanceled                float64 `stm:"build_canceled"`
			BuildTargetNotReachableError float64 `stm:"build_target_not_reachable_error"`
			CommandNotSupportedError     float64 `stm:"command_not_supported_error"`
			DockerfileEmptyError         float64 `stm:"dockerfile_empty_error"`
			DockerfileSyntaxError        float64 `stm:"dockerfile_syntax_error"`
			ErrorProcessingCommandsError float64 `stm:"error_processing_commands_error"`
			MissingOnbuildArgumentsError float64 `stm:"missing_onbuild_arguments_error"`
			UnknownInstructionError      float64 `stm:"unknown_instruction_error"`
		} `stm:"fails"`
	} `stm:"builder"`
	HealthChecks struct {
		Failed float64 `stm:"failed"`
	} `stm:"health_checks"`
	SwarmManager *swarmManager `stm:"swarm_manager"`
}

type containerStates struct {
	Paused  float64 `stm:"paused"`
	Running float64 `stm:"running"`
	Stopped float64 `stm:"stopped"`
}

type swarmManager struct {
	IsLeader float64 `stm:"leader"`
	Configs  float64 `stm:"configs_total"`
	Networks float64 `stm:"networks_total"`
	Secrets  float64 `stm:"secrets_total"`
	Services float64 `stm:"services_total"`
	Nodes    struct {
		Total    float64 `stm:"total"`
		PerState struct {
			Disconnected float64 `stm:"disconnected"`
			Down         float64 `stm:"down"`
			Ready        float64 `stm:"ready"`
			Unknown      float64 `stm:"unknown"`
		} `stm:"state"`
	} `stm:"nodes"`
	Tasks struct {
		Total    float64 `stm:"total"`
		PerState struct {
			Accepted  float64 `stm:"accepted"`
			Assigned  float64 `stm:"assigned"`
			Complete  float64 `stm:"complete"`
			Failed    float64 `stm:"failed"`
			New       float64 `stm:"new"`
			Orphaned  float64 `stm:"orphaned"`
			Pending   float64 `stm:"pending"`
			Preparing float64 `stm:"preparing"`
			Ready     float64 `stm:"ready"`
			Rejected  float64 `stm:"rejected"`
			Remove    float64 `stm:"remove"`
			Running   float64 `stm:"running"`
			Shutdown  float64 `stm:"shutdown"`
			Starting  float64 `stm:"starting"`
		} `stm:"state"`
	} `stm:"tasks"`
}
