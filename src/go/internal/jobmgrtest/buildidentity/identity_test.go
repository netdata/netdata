package buildidentity

import "testing"

func TestBuildTagCanonicalization(t *testing.T) {
	tests := map[string]struct {
		value string
		want  string
		has   bool
	}{
		"comma separated": {
			value: "netdata,ibm_mq", want: "ibm_mq,netdata", has: true,
		},
		"space separated": {
			value: "ibm_mq netdata", want: "ibm_mq,netdata", has: true,
		},
		"absent": {
			value: "netdata", want: "netdata",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if got := canonicalBuildTags(test.value); got != test.want {
				t.Fatalf("tags=%q, want %q", got, test.want)
			}
			if got := containsBuildTag(
				test.value,
				requiredIBMBuildTag,
			); got != test.has {
				t.Fatalf("contains ibm_mq=%v, want %v", got, test.has)
			}
		})
	}
}

func TestManifestExecutablePathIsContained(t *testing.T) {
	tests := map[string]struct {
		name    string
		wantErr bool
	}{
		"parent": {
			name: "../ibm.d.plugin", wantErr: true,
		},
		"absolute": {
			name: "/ibm.d.plugin", wantErr: true,
		},
		"backslash": {
			name: `bin\\ibm.d.plugin`, wantErr: true,
		},
		"non canonical": {
			name: "bin/../ibm.d.plugin", wantErr: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := containedManifestPath(t.TempDir(), test.name)
			if (err != nil) != test.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, test.wantErr)
			}
		})
	}
}
