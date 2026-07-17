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

func (*acknowledgedCommandPort) Submit(context.Context, jobmgr.Request) error {
	return nil
}

func (*acknowledgedCommandPort) SubmitAndWait(
	context.Context,
	jobmgr.Request,
) error {
	return nil
}

var _ CommandPort = (*acknowledgedCommandPort)(nil)

func TestCommandPortRequiresAcknowledgedSubmission(t *testing.T) {
	portType := reflect.TypeOf((*CommandPort)(nil)).Elem()
	_, ok := portType.MethodByName("SubmitAndWait")
	assert.True(t, ok)
}
