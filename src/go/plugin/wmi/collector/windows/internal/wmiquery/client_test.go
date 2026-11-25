// SPDX-License-Identifier: GPL-3.0-or-later

package wmiquery

import (
	"context"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/oiweiwei/go-msrpc/dcerpc"
	"github.com/oiweiwei/go-msrpc/msrpc/dcetypes"
	"github.com/oiweiwei/go-msrpc/msrpc/dcom"
	activation "github.com/oiweiwei/go-msrpc/msrpc/dcom/iactivation/v0"
	remoteunknown "github.com/oiweiwei/go-msrpc/msrpc/dcom/iremunknown/v0"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTCPBindingsUseOnlyAdvertisedPorts(t *testing.T) {
	var wire []uint16
	for _, binding := range []struct {
		tower int
		addr  string
	}{
		{dcetypes.ProtocolNamedPipe, "server[\\pipe\\ignored]"},
		{dcetypes.ProtocolTCP, "different-interface.example.test[49154]"},
		{dcetypes.ProtocolTCP, "different-interface.example.test[0]"},
		{dcetypes.ProtocolTCP, "different-interface.example.test"},
	} {
		wire = append(wire, uint16(binding.tower))
		wire = append(wire, utf16.Encode([]rune(binding.addr))...)
		wire = append(wire, 0)
	}
	wire = append(wire, 0)
	bindings := &dcom.DualStringArray{StringArray: wire, SecurityOffset: uint16(len(wire))}
	options := append(tcpEndpoints(bindings),
		dcerpc.WithInsecure(), dcerpc.WithAbstractSyntax(activation.ActivationSyntaxV0_0))
	parsed, err := dcerpc.ParseOptions(t.Context(), options...)
	require.NoError(t, err)
	assert.Equal(t, []string{"ncacn_ip_tcp:[49154]"}, parsed.Bindings)

	bindings.SecurityOffset++
	assert.Empty(t, tcpEndpoints(bindings))
}

func TestCloseReleasesReferencesBeforeCanceling(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	releaser := &fakeReleaser{t: t}
	client := &Client{
		ctx:     ctx,
		cancel:  cancel,
		unknown: releaser,
		refs: []*dcom.RemoteInterfaceReference{
			{IPID: &dcom.IPID{Data1: 1}, PublicReferencesCount: 5},
			{IPID: &dcom.IPID{Data1: 2}, PublicReferencesCount: 1},
		},
	}
	client.Close()
	client.Close()
	require.Len(t, releaser.requests, 1)
	req := releaser.requests[0]
	assert.EqualValues(t, 2, req.InterfaceReferencesCount)
	require.Len(t, req.InterfaceReferences, 2)
	assert.EqualValues(t, 5, req.InterfaceReferences[0].PublicReferencesCount)
	assert.EqualValues(t, 1, req.InterfaceReferences[1].PublicReferencesCount)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}

type fakeReleaser struct {
	remoteunknown.RemoteUnknownClient
	t        *testing.T
	requests []*remoteunknown.RemoteReleaseRequest
}

func (f *fakeReleaser) RemoteRelease(
	ctx context.Context, req *remoteunknown.RemoteReleaseRequest, _ ...dcerpc.CallOption,
) (*remoteunknown.RemoteReleaseResponse, error) {
	require.NoError(f.t, ctx.Err())
	deadline, ok := ctx.Deadline()
	require.True(f.t, ok)
	assert.LessOrEqual(f.t, time.Until(deadline), time.Second)
	f.requests = append(f.requests, req)
	return &remoteunknown.RemoteReleaseResponse{}, nil
}
