// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

type jobConfigIntentTransition uint8

const (
	jobConfigIntentPreserve jobConfigIntentTransition = iota
	jobConfigIntentEnable
	jobConfigIntentDisable
)

// Admission failures are separate from resource-owner failures so callers can
// reject malformed input or retry containment without granting acquisition.
type configIntentAdmissionError struct{ cause error }

func (e *configIntentAdmissionError) Error() string { return e.cause.Error() }
func (e *configIntentAdmissionError) Unwrap() error { return e.cause }

type enabledJobConfigIntent struct {
	identity collectorapi.JobConfigIdentity
	owner    collectorapi.JobConfigIntentOwner
}

// The candidate controller owns configuration; this index retains only enabled
// resource identity and its owner, independently of public graph visibility.
type jobConfigIntentIndex struct {
	mu      sync.Mutex
	enabled map[string]enabledJobConfigIntent
}

type preparedJobConfigIntent struct {
	index       *jobConfigIntentIndex
	id          string
	next        enabledJobConfigIntent
	reservation collectorapi.PreparedJobConfigIntent
	consumed    bool
}

func (p *preparedJobConfigIntent) commit() {
	if p == nil || p.consumed {
		return
	}
	// Adoption already happened. A panic must not abort its protection.
	p.consumed = true
	p.index.mu.Lock()
	if p.next.identity.Valid() {
		if p.index.enabled == nil {
			p.index.enabled = make(map[string]enabledJobConfigIntent)
		}
		p.index.enabled[p.id] = p.next
	} else {
		delete(p.index.enabled, p.id)
	}
	p.index.mu.Unlock()
	p.reservation.Commit()
}

func (p *preparedJobConfigIntent) abort() (err error) {
	if p == nil || p.consumed {
		return nil
	}
	p.consumed = true
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in config intent abort", lifecycle.ErrTaskPanic)
		}
		if err != nil {
			err = lifecycle.RetainOwnership(err)
		}
	}()
	return p.reservation.Abort()
}

func (dcjc *DynCfgJobController) prepareJobConfigIntent(
	ctx context.Context,
	id string,
	config confgroup.Config,
	transition jobConfigIntentTransition,
) (*preparedJobConfigIntent, error) {
	dcjc.configIntents.mu.Lock()
	previous := dcjc.configIntents.enabled[id]
	dcjc.configIntents.mu.Unlock()
	if config == nil {
		transition = jobConfigIntentDisable
	}
	enabled := previous.identity.Valid()
	switch transition {
	case jobConfigIntentEnable:
		enabled = true
	case jobConfigIntentDisable:
		enabled = false
	case jobConfigIntentPreserve:
	default:
		return nil, errors.New("job output: invalid config intent transition")
	}
	var next enabledJobConfigIntent
	if enabled {
		creator, ok := dcjc.modules.Lookup(config.Module())
		if ok && !nilInterfaceValue(creator.JobConfigIntent) {
			next = enabledJobConfigIntent{
				identity: jobConfigIdentity(config),
				owner:    creator.JobConfigIntent,
			}
		}
	}
	if previous.identity == next.identity {
		return nil, nil
	}
	owner := next.owner
	if owner == nil {
		owner = previous.owner
	}
	if owner == nil {
		return nil, nil
	}
	prepare := func(config confgroup.Config) configOperationResult {
		reservation, err := prepareConfigIntentOwner(owner, previous.identity, next.identity, config)
		if err != nil {
			return configOperationResult{err: err}
		}
		return configOperationResult{intent: &preparedJobConfigIntent{
			index: &dcjc.configIntents, id: id, next: next, reservation: reservation,
		}}
	}
	if !next.identity.Valid() {
		result := prepare(config)
		return result.intent, result.err
	}
	result, err := dcjc.runPreparedConfigOperation(ctx, config, configOperationValidateIntent, true,
		func(ctx context.Context, config confgroup.Config) configOperationResult {
			if err := dcjc.factory.ValidateConfig(ctx, config); err != nil {
				return configOperationResult{err: &configIntentAdmissionError{cause: err}}
			}
			if cause := context.Cause(ctx); cause != nil {
				return configOperationResult{err: &configIntentAdmissionError{cause: cause}}
			}
			return prepare(config)
		})
	if err != nil {
		return nil, &configIntentAdmissionError{cause: err}
	}
	return result.intent, result.err
}

// Resource demand follows the admitted binding, including hidden Failed stock.
// Public graph visibility and acquisition authority are separate projections.
func (dcjc *DynCfgJobController) prepareMutationJobConfigIntent(
	ctx context.Context, id string, postimage *dyncfg.GraphConfig, reply jobReply, transition jobConfigIntentTransition,
) (*preparedJobConfigIntent, error) {
	var config confgroup.Config
	if delta := reply.candidate; delta != nil {
		after := delta.after
		if after.Binding != nil {
			config = after.Binding.Config
		}
		if after.Enablement != candidateEnabled || after.Manual == candidateManualSourceRevoked || config == nil {
			transition = jobConfigIntentDisable
		} else {
			transition = jobConfigIntentEnable
		}
	} else if postimage != nil {
		creator, exists := dcjc.modules.Lookup(postimage.Module)
		if transition == jobConfigIntentDisable || postimage.Status == dyncfg.StatusDisabled.String() ||
			!exists || nilInterfaceValue(creator.JobConfigIntent) {
			return dcjc.prepareJobConfigIntent(ctx, id, nil, jobConfigIntentDisable)
		}
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil || config == nil ||
			config.Module() != postimage.Module || config.Name() != postimage.Name {
			return nil, errors.New("job output: invalid enabled config intent payload")
		}
	}
	return dcjc.prepareJobConfigIntent(ctx, id, config, transition)
}

func prepareConfigIntentOwner(
	owner collectorapi.JobConfigIntentOwner,
	previous, next collectorapi.JobConfigIdentity,
	config confgroup.Config,
) (reservation collectorapi.PreparedJobConfigIntent, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = lifecycle.RetainOwnership(fmt.Errorf("%w in config intent preparation", lifecycle.ErrTaskPanic))
		}
	}()
	reservation, err = owner.Prepare(previous, next, map[string]any(config))
	if err != nil {
		if !nilInterfaceValue(reservation) {
			cleanup := &preparedJobConfigIntent{
				reservation: reservation,
			}
			err = errors.Join(err, cleanup.abort())
		}
		return nil, err
	}
	if nilInterfaceValue(reservation) {
		return nil, errors.New("job output: config intent owner returned no reservation")
	}
	return reservation, nil
}

// FinalizeJobConfigIntents retires committed enabled resources after all jobs,
// candidate attempts and graph mutations in this run have joined. A successor run
// restores its own enabled demand; gaps during replay follow cache retention.
// Failed retirement remains indexed and may be retried; it is never fail-open.
func (dcjc *DynCfgJobController) FinalizeJobConfigIntents() error {
	if dcjc == nil {
		return nil
	}
	dcjc.configIntents.mu.Lock()
	ids := make([]string, 0, len(dcjc.configIntents.enabled))
	for id := range dcjc.configIntents.enabled {
		ids = append(ids, id)
	}
	dcjc.configIntents.mu.Unlock()
	var result error
	for _, id := range ids {
		prepared, err := dcjc.prepareJobConfigIntent(context.Background(), id, nil, jobConfigIntentDisable)
		if err != nil {
			result = errors.Join(result, err)
			continue
		}
		prepared.commit()
	}
	return result
}

// Satisfied Enable still adopts enabled intent. Preserve the existing no-op
// diagnostic and retry behavior: graph equality alone is not an intent commit.
func (dcjc *DynCfgJobController) satisfyWithEnabledIntent(
	ctx context.Context,
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	record dyncfg.GraphRecord,
	afterApply func(),
) (lifecycle.PreparedResourceTransaction, error) {
	status := dyncfg.Status(record.Status)
	postimage := graphConfig(record, status)
	intent, err := dcjc.prepareMutationJobConfigIntent(ctx, scope.ID, &postimage, internalReply(), jobConfigIntentEnable)
	if err != nil {
		return nil, err
	}
	if intent == nil {
		return dcjc.satisfy(scope, current, permit, status, afterApply)
	}
	transaction, err := dcjc.prepareResourceTransaction(ResourceTransactionSpec{
		Scope:        scope,
		Disposition:  lifecycle.ResourceTransactionUnchanged,
		Current:      current,
		UnusedPermit: permit,
		configIntent: intent,
		AfterApply:   afterApply,
		Result:       satisfiedResult(status),
		Cleanup:      dcjc.configStatusCleanup(scope.ID, status),
	})
	if err != nil {
		return nil, errors.Join(err, intent.abort())
	}
	return transaction, nil
}
