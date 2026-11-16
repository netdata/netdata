// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import "context"

// Admission failures reject malformed bindings or wait for their contained
// validation attempt without adopting an unvalidated dependency waiter.
type configAdmissionError struct{ cause error }

func (e *configAdmissionError) Error() string { return e.cause.Error() }
func (e *configAdmissionError) Unwrap() error { return e.cause }

func (dcjc *DynCfgJobController) validateCandidateAdmission(ctx context.Context, delta *candidateDelta) error {
	if delta == nil || delta.after.Enablement != candidateEnabled ||
		delta.after.Manual == candidateManualSourceRevoked || delta.after.Binding == nil {
		return nil
	}
	config := delta.after.Binding.Config
	creator, exists := dcjc.modules.Lookup(config.Module())
	if !exists || creator.ActivationDependencies == nil {
		return nil
	}
	// Health/retry changes retain an already admitted binding. Only a changed
	// or newly enabled binding needs validation before dependency readiness can
	// cause construction to return without applying its configuration.
	if before := delta.before; before != nil && before.Enablement == candidateEnabled &&
		before.Manual != candidateManualSourceRevoked && before.Binding != nil &&
		jobConfigIdentity(before.Binding.Config) == jobConfigIdentity(config) {
		return nil
	}
	if _, err := dcjc.runConfigOperation(ctx, config, configOperationAdmit, true); err != nil {
		return &configAdmissionError{cause: err}
	}
	return nil
}
