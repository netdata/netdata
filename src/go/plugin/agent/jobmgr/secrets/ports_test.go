// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"reflect"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/stretchr/testify/assert"
)

type acknowledgedCommandPort struct{}

func (*acknowledgedCommandPort) SubmitPrepared(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}

func (*acknowledgedCommandPort) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}

var _ CommandPort = (*acknowledgedCommandPort)(nil)

func TestCommandPortRequiresAcknowledgedPreparedSubmission(t *testing.T) {
	portType := reflect.TypeFor[CommandPort]()
	_, ok := portType.MethodByName("SubmitPreparedAndWait")
	assert.True(t, ok)
}
