// SPDX-License-Identifier: GPL-3.0-or-later

package acquisition

import (
	"encoding/json"
	"net/url"
	"testing"
)

func FuzzDecodeJSONBytes(f *testing.F) {
	f.Add([]byte(`{"Reading":0,"Name":"sensor","Status":{"Health":"OK"}}`))
	f.Add([]byte(`{"Reading":1e300,"Null":null,"Array":[true,false]}`))
	f.Add([]byte(`null`))
	f.Add([]byte(`{`))

	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) > maxResponseBodyBytes {
			return
		}
		var decoded any
		if err := decodeJSONBytes(raw, &decoded); err != nil {
			return
		}
		encoded, err := json.Marshal(decoded)
		if err != nil {
			t.Fatalf("marshal accepted JSON: %v", err)
		}
		var roundTrip any
		if err := decodeJSONBytes(encoded, &roundTrip); err != nil {
			t.Fatalf("decode re-encoded JSON: %v", err)
		}
	})
}

func FuzzResolveRedfishURI(f *testing.F) {
	for _, raw := range []string{
		"/redfish/v1/Systems/1",
		"/redfish/v1/Systems?$skip=1",
		"/redfish/v1/Chassis/1/Sensors/1#/Reading",
		"https://example.com/redfish/v1/Systems",
		"/redfish/v1/%2e%2e/admin",
		"Systems/1",
	} {
		f.Add(raw)
	}

	root, err := url.Parse("https://192.0.2.1/redfish/v1/")
	if err != nil {
		f.Fatal(err)
	}
	const origin = "https://192.0.2.1"

	f.Fuzz(func(t *testing.T, raw string) {
		if len(raw) > maxURIBytes {
			return
		}
		ref, parseErr := url.Parse(raw)
		for _, mode := range []redfishURIMode{uriResource, uriOpaquePage, uriProvenance} {
			target, err := resolveRedfishURI(origin, root, raw, mode)
			if err != nil {
				continue
			}
			if parseErr != nil {
				t.Fatalf("accepted an unparseable URI %q", raw)
			}
			switch mode {
			case uriResource:
				if ref.RawQuery != "" || ref.Fragment != "" || target.RawQuery != "" || target.Fragment != "" {
					t.Fatalf("resource mode accepted query or fragment in %q as %q", raw, target.String())
				}
			case uriOpaquePage:
				if ref.Fragment != "" || target.Fragment != "" {
					t.Fatalf("page mode accepted fragment in %q as %q", raw, target.String())
				}
			case uriProvenance:
				if ref.RawQuery != "" || target.RawQuery != "" {
					t.Fatalf("provenance mode accepted query in %q as %q", raw, target.String())
				}
				if !validJSONPointerFragment(ref.Fragment) || !validJSONPointerFragment(target.Fragment) {
					t.Fatalf("provenance mode accepted invalid JSON Pointer fragment in %q as %q", raw, target.String())
				}
			}
			if target.Scheme+"://"+target.Host != origin {
				t.Fatalf("accepted cross-origin URI %q as %q", raw, target.String())
			}
			if target.User != nil {
				t.Fatalf("accepted user-info in URI %q", raw)
			}
			if len(target.Path) < len("/redfish/") || target.Path[:len("/redfish/")] != "/redfish/" {
				t.Fatalf("accepted non-Redfish URI %q as %q", raw, target.String())
			}
		}
	})
}
