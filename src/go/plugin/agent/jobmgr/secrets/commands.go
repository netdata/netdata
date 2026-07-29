// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

const (
	// A rendered response is capped at maximumSecretJobSummaryBytes. Each job-name
	// list gets the content budget below, reserving secretJobSummaryReserveBytes
	// for surrounding prose and its truncation marker; boundSecretMessage enforces
	// the total after lists combine.
	maximumSecretJobSummaryBytes = 4 * 1024
	secretJobSummaryReserveBytes = 512
	secretJobSummaryContentBytes = maximumSecretJobSummaryBytes - secretJobSummaryReserveBytes
)

// Operator-facing response messages reused across secretstore commands.
const (
	msgSecretStoreNotConfigured    = "The specified secretstore '%s' is not configured."
	msgInvalidSecretStoreConfig    = "Invalid secretstore configuration."
	msgSecretStoreValidationFailed = "Secretstore configuration validation failed."
)

func (c *Controller) Stage(input CommandInput) (*PreparedStoreOperation, error) {
	if c == nil || c.operations == nil {
		return nil, errors.New("jobmgr secrets: invalid Store operation staging")
	}
	c.mu.Lock()
	commandsReady := c.commandsReady
	c.mu.Unlock()
	if !commandsReady {
		return c.operations.immediate(storeOperationResult{}), nil
	}
	target, failure := c.resolveTarget(input)
	if failure != nil {
		return c.operations.immediate(storeOperationResult{}), nil
	}
	spec := storeOperationSpec{
		target: target,
		input:  input,
	}
	var versionErr error
	switch target.command {
	case dyncfg.CommandAdd:
		spec.mode = storeOperationMutation
		spec.expected = c.store.Generation(target.key)
		spec.supersede = true
		spec.desiredVersion, versionErr = c.allocateDesiredVersion()
	case dyncfg.CommandUpdate:
		if _, ok := c.entry(target.key); !ok {
			return c.operations.immediate(storeOperationResult{}), nil
		}
		spec.mode = storeOperationMutation
		spec.expected = c.store.Generation(target.key)
		spec.supersede = true
		spec.desiredVersion, versionErr = c.allocateDesiredVersion()
	case dyncfg.CommandTest:
		entry, ok := c.entry(target.key)
		if !ok {
			return c.operations.immediate(storeOperationResult{}), nil
		}
		spec.mode = storeOperationValidation
		spec.testIdentity = true
		spec.validationOnly = !input.HasPayload
		if spec.validationOnly {
			spec.config = entry.config
		}
	case dyncfg.CommandRemove:
		spec.mode = storeOperationRemoval
		spec.desiredVersion, versionErr = c.allocateDesiredVersion()
	default:
		return c.operations.immediate(storeOperationResult{}), nil
	}
	if versionErr != nil {
		return nil, versionErr
	}
	return c.operations.prepare(spec)
}

func (c *Controller) prepareSchema(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	if target.key != "" {
		if _, ok := c.entry(target.key); !ok {
			return c.noopMessage(scope, current, 404, fmt.Sprintf(msgSecretStoreNotConfigured, target.key))
		}
	}
	schema, ok := c.creators.Schema(target.kind)
	if !ok {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf("The specified secretstore kind '%s' is not supported.", target.kind),
		)
	}
	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(schema))
	if err != nil {
		return nil, err
	}
	return c.noop(scope, current, result, nil, nil)
}

func (c *Controller) prepareGet(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	entry, ok := c.entry(target.key)
	if !ok {
		return c.noopMessage(scope, current, 404, fmt.Sprintf(msgSecretStoreNotConfigured, target.key))
	}
	typed, err := typedSecretConfig(c.creators, entry.config.Kind())
	if err == nil {
		var payload []byte
		payload, err = yaml.Marshal(entry.config)
		if err == nil {
			err = yaml.Unmarshal(payload, typed)
		}
	}
	if err != nil {
		return c.noopMessage(scope, current, 500, "Failed to materialize secretstore configuration.")
	}
	payload, err := json.Marshal(typed)
	if err != nil {
		return nil, err
	}
	result, err := lifecycle.NewSealedResult(200, "application/json", payload)
	if err != nil {
		return nil, err
	}
	return c.noop(scope, current, result, nil, nil)
}

func (c *Controller) prepareUserConfig(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	input CommandInput,
	target secretTarget,
) (lifecycle.PreparedResourceTransaction, error) {
	typed, err := typedSecretConfig(c.creators, target.kind)
	if err == nil {
		err = parseSecretPayload(input, typed)
	}
	if err != nil {
		return c.noopMessage(scope, current, 400, msgInvalidSecretStoreConfig)
	}
	payload, err := yaml.Marshal(typed)
	if err != nil {
		return nil, err
	}
	result, err := lifecycle.NewSealedResult(200, "application/yaml", payload)
	if err != nil {
		return nil, err
	}
	return c.noop(scope, current, result, nil, nil)
}

func (c *Controller) prepareTest(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
	stage *PreparedStoreOperation,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	entry, ok := c.entry(target.key)
	if !ok {
		return c.noopMessage(scope, current, 404, fmt.Sprintf(msgSecretStoreNotConfigured, target.key))
	}
	operation, err := takeStoreOperation(stage)
	if err != nil {
		return nil, err
	}
	defer operation.releaseUntransferred(&transaction, &resultErr)
	result := operation.result
	if result.retryable {
		return c.noopMessage(scope, current, 503, "Secretstore test is still busy.")
	}
	if result.config == nil {
		return c.noopMessage(scope, current, 400, msgInvalidSecretStoreConfig)
	}
	if result.err != nil {
		return c.noopMessage(scope, current, 400, msgSecretStoreValidationFailed)
	}
	if !result.validationOnly && result.config.Hash() == entry.config.Hash() {
		return c.noopMessage(scope, current, 202, "Submitted configuration does not change the active secretstore.")
	}
	affected := formatSecretJobs(c.dependencies.Affected(target.key, false))
	restartable := formatSecretJobs(c.dependencies.Affected(target.key, true))
	return c.noopMessage(
		scope,
		current,
		202,
		secretImpactMessage(affected, restartable, result.validationOnly),
	)
}

func (c *Controller) prepareAdd(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
	stage *PreparedStoreOperation,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	operation, err := takeStoreOperation(stage)
	if err != nil {
		return nil, err
	}
	defer operation.releaseUntransferred(&transaction, &resultErr)
	result := operation.result
	if result.config == nil {
		return c.noopMessageWithCommit(
			scope,
			current,
			400,
			msgInvalidSecretStoreConfig,
			func() {
				c.clearPendingThrough(target.key, result.desiredVersion)
			},
		)
	}
	config := result.config
	entry, exists := c.entry(target.key)
	expected := c.store.Generation(target.key)
	if expected != 0 {
		if !exists || current == nil || !scope.Current.Valid() {
			return nil, errors.New("jobmgr secrets: active Store differs from command resource")
		}
	} else if current != nil || scope.Current.Valid() {
		return nil, errors.New("jobmgr secrets: command resource has no active Store")
	}
	if expected != 0 &&
		entry.status == dyncfg.StatusRunning &&
		entry.config.SourceType() == confgroup.TypeDyncfg &&
		entry.config.Hash() == config.Hash() {
		return c.noopWithCommit(
			scope,
			current,
			mustSecretMessage(200, ""),
			nil,
			c.configCreateCleanup(entry),
			func() {
				c.clearPendingThrough(target.key, result.desiredVersion)
			},
		)
	}
	if result.expected != expected {
		result.retryable = true
		result.err = errors.New("jobmgr secrets: Store changed while preparation was staged")
		return c.prepareRetryableResult(scope, current, result, expected == 0)
	}
	if result.retryable {
		return c.prepareRetryableResult(scope, current, result, expected == 0)
	}
	return c.prepareStoreMutation(scope, current, operation, expected == 0)
}

func (c *Controller) prepareUpdate(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
	stage *PreparedStoreOperation,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	entry, exists := c.entry(target.key)
	if !exists {
		return c.noopMessage(
			scope,
			current,
			404,
			fmt.Sprintf(msgSecretStoreNotConfigured, target.key),
		)
	}
	operation, err := takeStoreOperation(stage)
	if err != nil {
		return nil, err
	}
	defer operation.releaseUntransferred(&transaction, &resultErr)
	result := operation.result
	if result.config == nil {
		return c.noopMessageWithCommit(
			scope,
			current,
			400,
			msgInvalidSecretStoreConfig,
			func() {
				c.clearPendingThrough(target.key, result.desiredVersion)
			},
		)
	}
	config := result.config
	expected := c.store.Generation(target.key)
	if expected != 0 {
		if current == nil || !scope.Current.Valid() {
			return nil, errors.New("jobmgr secrets: active Store differs from command resource")
		}
	} else if current != nil || scope.Current.Valid() {
		return nil, errors.New("jobmgr secrets: command resource has no active Store")
	}
	installFailure := expected == 0
	if expected != 0 && entry.config.Hash() == config.Hash() {
		return c.noopWithCommit(
			scope,
			current,
			mustSecretMessage(200, ""),
			nil,
			c.configCreateCleanup(entry),
			func() {
				c.clearPendingThrough(target.key, result.desiredVersion)
			},
		)
	}
	if result.expected != expected {
		result.retryable = true
		result.err = errors.New("jobmgr secrets: Store changed while preparation was staged")
		return c.prepareRetryableResult(scope, current, result, installFailure)
	}
	if result.retryable {
		return c.prepareRetryableResult(scope, current, result, installFailure)
	}
	return c.prepareStoreMutation(scope, current, operation, installFailure)
}

func (c *Controller) prepareRemove(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	target secretTarget,
	stage *PreparedStoreOperation,
) (
	transaction lifecycle.PreparedResourceTransaction,
	resultErr error,
) {
	operation, err := takeStoreOperation(stage)
	if err != nil {
		return nil, err
	}
	defer operation.releaseUntransferred(&transaction, &resultErr)
	result := operation.result
	if !result.removal {
		return nil, errors.New("jobmgr secrets: removal did not pass its pre-claim stage")
	}
	entry, exists := c.entry(target.key)
	if !exists {
		return c.noopMessage(scope, current, 404, fmt.Sprintf(msgSecretStoreNotConfigured, target.key))
	}
	if affected := formatSecretJobs(c.dependencies.Affected(target.key, false)); affected != "" {
		return c.noopMessage(
			scope,
			current,
			409,
			fmt.Sprintf("The specified secretstore '%s' is used by jobs (%s).", target.key, affected),
		)
	}
	if entry.config.SourceType() != confgroup.TypeDyncfg {
		return c.noopMessage(
			scope,
			current,
			405,
			fmt.Sprintf(
				"removing configurations of source type '%s' is not supported, only 'dyncfg' configurations can be removed.",
				entry.config.SourceType(),
			),
		)
	}
	expected := c.store.Generation(target.key)
	if expected == 0 &&
		current == nil &&
		!scope.Current.Valid() &&
		entry.status == dyncfg.StatusFailed {
		return newPreparedSecretTransaction(
			preparedSecretSpec{
				scope:      scope,
				current:    current,
				storeKey:   target.key,
				remove:     true,
				result:     mustSecretMessage(200, ""),
				cleanup:    c.configDeleteCleanup(target.key),
				controller: c,
				commit: func() {
					c.clearPendingThrough(target.key, result.desiredVersion)
				},
			},
		)
	}
	if expected == 0 || current == nil || !scope.Current.Valid() {
		return c.noopMessage(scope, current, 409, "Secretstore has no active generation.")
	}
	mutation, err := c.store.PrepareRemoval(target.key, expected)
	if err != nil {
		return nil, err
	}
	mutationOwner := preparedMutationOwner{mutation: mutation}
	defer mutationOwner.releaseUntransferred(&transaction, &resultErr)
	return mutationOwner.prepareTransaction(preparedSecretSpec{
		scope:      scope,
		current:    current,
		store:      c.store,
		storeKey:   target.key,
		remove:     true,
		result:     mustSecretMessage(200, ""),
		cleanup:    c.configDeleteCleanup(target.key),
		controller: c,
		commit: func() {
			c.clearPendingThrough(target.key, result.desiredVersion)
		},
	})
}

func materializeSecretConfig(input CommandInput, target secretTarget) (secretstore.Config, error) {
	var config secretstore.Config
	if err := parseSecretPayload(input, &config); err != nil {
		return nil, err
	}
	if config == nil {
		config = secretstore.Config{}
	}
	config.SetName(target.name)
	config.SetKind(target.kind)
	config.SetSource(confgroup.TypeDyncfg)
	config.SetSourceType(confgroup.TypeDyncfg)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return config, nil
}

func (c *Controller) prepareStoreMutation(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	operation *takenStoreOperation,
	installFailure bool,
) (lifecycle.PreparedResourceTransaction, error) {
	if operation == nil || !scope.Successor.Valid() {
		return nil, errors.New("jobmgr secrets: invalid Store mutation successor")
	}
	materialized := operation.result
	config := materialized.config
	prepareErr := materialized.err
	entry := secretEntry{
		config: config,
		status: dyncfg.StatusRunning,
	}
	if prepareErr != nil {
		spec := preparedSecretSpec{
			scope:      scope,
			current:    current,
			store:      c.store,
			storeKey:   config.ExposedKey(),
			result:     mustSecretMessage(400, msgSecretStoreValidationFailed),
			cleanup:    func() error { return nil },
			controller: c,
			commit: func() {
				c.clearPendingThrough(config.ExposedKey(), materialized.desiredVersion)
			},
		}
		if installFailure {
			entry.status = dyncfg.StatusFailed
			spec.cleanup = c.configCreateCleanup(entry)
			spec.entry = &entry
		}
		if operation.mutation.mutation.Valid() {
			spec.abort = true
			return operation.mutation.prepareTransaction(spec)
		}
		return newPreparedSecretTransaction(spec)
	}
	return operation.mutation.prepareTransaction(preparedSecretSpec{
		scope:      scope,
		current:    current,
		store:      c.store,
		storeKey:   config.ExposedKey(),
		result:     mustSecretMessage(200, ""),
		cleanup:    c.configCreateCleanup(entry),
		controller: c,
		entry:      &entry,
		restarts:   c.restartCommand(),
		commit: func() {
			c.clearPendingThrough(config.ExposedKey(), materialized.desiredVersion)
		},
	})
}

func takeStoreOperation(stage *PreparedStoreOperation) (*takenStoreOperation, error) {
	if stage == nil {
		return nil, errors.New("jobmgr secrets: Store command has no pre-claim stage")
	}
	result, err := stage.take()
	if err != nil {
		return nil, err
	}
	mutation := result.mutation
	result.mutation = nil
	return &takenStoreOperation{
		result:   result,
		mutation: preparedMutationOwner{mutation: mutation},
	}, nil
}

func (c *Controller) restartCommand() *SecretRestartCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.restarts
}

func (c *Controller) noop(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	result lifecycle.SealedResult,
	entry *secretEntry,
	cleanup lifecycle.TaskCleanup,
) (lifecycle.PreparedResourceTransaction, error) {
	return c.noopWithCommit(scope, current, result, entry, cleanup, nil)
}

func (c *Controller) noopWithCommit(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	result lifecycle.SealedResult,
	entry *secretEntry,
	cleanup lifecycle.TaskCleanup,
	commit func(),
) (lifecycle.PreparedResourceTransaction, error) {
	if cleanup == nil {
		cleanup = func() error { return nil }
	}
	return newPreparedSecretTransaction(
		preparedSecretSpec{
			scope:      scope,
			current:    current,
			result:     result,
			cleanup:    cleanup,
			controller: c,
			entry:      entry,
			commit:     commit,
		},
	)
}

func (c *Controller) noopMessage(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	code int,
	message string,
) (lifecycle.PreparedResourceTransaction, error) {
	return c.noop(scope, current, mustSecretMessage(code, message), nil, nil)
}

func (c *Controller) noopMessageWithCommit(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	code int,
	message string,
	commit func(),
) (lifecycle.PreparedResourceTransaction, error) {
	return c.noopWithCommit(
		scope,
		current,
		mustSecretMessage(code, message),
		nil,
		nil,
		commit,
	)
}

func formatSecretJobs(refs []secretstore.JobRef) string {
	return formatBoundedSecretNames(refs, func(ref secretstore.JobRef) string {
		if ref.Display != "" {
			return ref.Display
		}
		return ref.ID
	})
}

func formatSecretJobNames(names []string) string {
	return formatBoundedSecretNames(names, func(name string) string { return name })
}

func formatBoundedSecretNames[T any](items []T, name func(T) string) string {
	if len(items) == 0 || name == nil {
		return ""
	}
	var builder strings.Builder
	builder.Grow(maximumSecretJobSummaryBytes)
	for index, item := range items {
		value := name(item)
		separatorBytes := 0
		if builder.Len() != 0 {
			separatorBytes = 2
		}
		if len(value) > secretJobSummaryContentBytes-builder.Len()-separatorBytes {
			if builder.Len() != 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("... and ")
			builder.WriteString(strconv.Itoa(len(items) - index))
			builder.WriteString(" more")
			break
		}
		if separatorBytes != 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(value)
	}
	return builder.String()
}

func secretImpactMessage(affected string, restartable string, validationOnly bool) string {
	var message string
	if validationOnly {
		if affected == "" {
			message = "Stored configuration is valid. No jobs are currently using this secretstore."
		} else if restartable == "" {
			message = "Stored configuration is valid. This secretstore is used by jobs: " +
				affected +
				". No running jobs would be restarted automatically by a change."
		} else {
			message = "Stored configuration is valid. This secretstore is used by jobs: " +
				affected +
				". Running jobs that would be restarted automatically by a change: " +
				restartable + "."
		}
	} else if affected == "" {
		message = "No jobs currently use this secretstore."
	} else if restartable == "" {
		message = "Updated configuration is used by jobs: " +
			affected +
			". No running jobs would be restarted automatically."
	} else {
		message = "Updated configuration is used by jobs: " +
			affected +
			". Running jobs that would be restarted automatically: " +
			restartable + "."
	}
	return boundSecretMessage(message)
}

func boundSecretMessage(message string) string {
	if len(message) <= maximumSecretJobSummaryBytes {
		return message
	}
	const suffix = "... [truncated]"
	end := maximumSecretJobSummaryBytes - len(suffix)
	for end > 0 && !utf8.RuneStart(message[end]) {
		end--
	}
	return message[:end] + suffix
}
