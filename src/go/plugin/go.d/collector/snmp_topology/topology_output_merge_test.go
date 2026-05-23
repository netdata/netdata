// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAppendUniqueTopologyStringsSortsAndDeduplicates(t *testing.T) {
	values := appendUniqueTopologyStrings([]string{" b ", "a"}, "a", "", " c ")
	require.Equal(t, []string{"a", "b", "c"}, values)
}

func TestMergeTopologyStringMapKeepsExistingAndIgnoresBlankEntries(t *testing.T) {
	base := map[string]string{
		"existing": "keep",
	}

	merged := mergeTopologyStringMap(base, map[string]string{
		"existing": "replace",
		" new ":    " value ",
		"blank":    " ",
		"":         "ignored",
	})

	require.Equal(t, map[string]string{
		"existing": "keep",
		"new":      "value",
	}, merged)
}

func TestMergeTopologyAnyMapKeepsExistingAndAddsMissingKeys(t *testing.T) {
	base := map[string]any{
		"existing": "keep",
	}

	merged := mergeTopologyAnyMap(base, map[string]any{
		"existing": "replace",
		" new ":    42,
		"":         "ignored",
	})

	require.Equal(t, "keep", merged["existing"])
	require.Equal(t, 42, merged["new"])
	require.NotContains(t, merged, "")
}

func TestTopologyLinkActorKeyIncludesStateAndAttachmentMode(t *testing.T) {
	base := topologyLink{
		Protocol:   "bridge",
		Direction:  "bidirectional",
		SrcActorID: "device:a",
		DstActorID: "endpoint:b",
		Src: topologyLinkEndpoint{Attributes: map[string]any{
			"if_name": "Gi0/1",
		}},
		Dst: topologyLinkEndpoint{Attributes: map[string]any{
			"if_name": "Gi0/2",
		}},
		Metrics: map[string]any{
			"bridge_domain":   "vlan-200",
			"attachment_mode": "probable_bridge_anchor",
			"inference":       "probable",
		},
		State: "probable",
	}

	strict := base
	strict.State = ""
	strict.Metrics = map[string]any{
		"bridge_domain": "vlan-200",
	}

	require.NotEqual(t, topologyLinkActorKey(base), topologyLinkActorKey(strict))
}
