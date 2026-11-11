// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/golangsnmp/gomib"
	"github.com/golangsnmp/gomib/mib"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestStockProfiles_OIDsResolveAgainstMIBs checks every OID in the stock profiles against a MIB corpus. It is opt-in
// because the repository ships no MIBs: set NETDATA_SNMP_MIB_DIRS to one or more MIB directories (separated by the OS
// path list separator), such as the mibs/ directory of a librenms/librenms checkout. References whose MIB object is
// not in the corpus are counted, not failed.
//
// It fails on:
//   - a column OID below a leaf object (a doubled entry arc or extra arcs);
//   - a column OID that names a scalar, table or row instead of a column;
//   - a scalar OID that names the object itself (no instance), or a scalar instance other than .0;
//   - a symbol name that is another object of the row's MIB module (an arc slip).
func TestStockProfiles_OIDsResolveAgainstMIBs(t *testing.T) {
	mibDirs := filepath.SplitList(os.Getenv("NETDATA_SNMP_MIB_DIRS"))
	if len(mibDirs) == 0 {
		t.Skip("NETDATA_SNMP_MIB_DIRS is not set")
	}
	corpus := loadMIBCorpus(t, mibDirs)

	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var verified, unresolved int
	for _, file := range files {
		content, err := os.ReadFile(file)
		require.NoError(t, err)
		var doc any
		require.NoError(t, yaml.Unmarshal(content, &doc), filepath.Base(file))

		var refs []profileOIDRef
		collectProfileOIDRefs(doc, "", "", false, &refs)
		for _, ref := range refs {
			problem, resolved := corpus.check(ref)
			if _, ok := acceptedMIBNameMismatches[filepath.Base(file)+" "+ref.oid]; ok {
				problem, resolved = "", true
			}
			switch {
			case problem != "":
				t.Errorf("%s %s (%s): %s", filepath.Base(file), ref.path, ref.oid, problem)
			case resolved:
				verified++
			default:
				unresolved++
			}
		}
	}
	t.Logf("%d OID references verified, %d not resolvable in the MIB corpus", verified, unresolved)
}

// acceptedMIBNameMismatches lists metric names that differ from the MIB object at their OID but are kept because the
// name is a published chart context and the data is right. Key: "<profile file> <OID>".
var acceptedMIBNameMismatches = map[string]string{
	"apc-ups.yaml 1.3.6.1.4.1.318.1.1.1.2.3.2.0": "upsHighPrecBatteryTemperature and the named " +
		"upsHighPrecExtdBatteryTemperature both report the internal UPS temperature in tenths of degrees Celsius",
}

type profileOIDRef struct {
	path   string
	oid    string
	name   string
	module string // the MIB named by the enclosing row
	column bool   // read by walking a table row, not by GET
}

// collectProfileOIDRefs walks raw profile YAML so each reference is attributed to the file that defines it.
func collectProfileOIDRefs(node any, path, module string, inTable bool, refs *[]profileOIDRef) {
	switch v := node.(type) {
	case map[any]any:
		if s, ok := v["MIB"].(string); ok {
			module = s
		}
		if table, ok := v["table"].(map[any]any); ok && table["OID"] != nil {
			inTable = true
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, fmt.Sprint(k))
		}
		slices.Sort(keys)
		for _, key := range keys {
			value := v[key]
			switch _, isMap := value.(map[any]any); {
			case key == "table" && isMap:
				// The table OID is the walk root, not a value source.
			case key == "OID" || key == "from":
				if oid, ok := value.(string); ok && isNumericOID(oid) {
					*refs = append(*refs, profileOIDRef{
						path:   path,
						oid:    oid,
						name:   profileSymbolName(v),
						module: module,
						column: inTable,
					})
				}
			default:
				collectProfileOIDRefs(value, path+"."+key, module, inTable, refs)
			}
		}
	case []any:
		for i, item := range v {
			collectProfileOIDRefs(item, fmt.Sprintf("%s[%d]", path, i), module, inTable, refs)
		}
	}
}

func profileSymbolName(m map[any]any) string {
	if s, ok := m["name"].(string); ok {
		return s
	}
	if s, ok := m["symbol"].(string); ok { // legacy tag form: {OID: ..., symbol: name}
		return s
	}
	return ""
}

func isNumericOID(s string) bool {
	_, err := mib.ParseOID(strings.TrimPrefix(s, "."))
	return err == nil
}

type mibCorpus struct {
	mib     *mib.Mib
	modules map[string]*mib.Module            // lower-case module name
	objects map[string]map[string]*mib.Object // lower-case module name -> OID -> object
}

func loadMIBCorpus(t *testing.T, dirs []string) *mibCorpus {
	t.Helper()

	var sources []gomib.Source
	for _, dir := range dirs {
		src, err := gomib.Dir(dir, gomib.WithExtensions("", ".mib", ".my", ".mi2", ".txt", ".smi"))
		require.NoError(t, err)
		sources = append(sources, src)
	}
	m, err := gomib.Load(context.Background(),
		gomib.WithSource(gomib.Multi(sources...)),
		gomib.WithResolverStrictness(mib.ResolverPermissive),
		gomib.WithDiagnosticConfig(mib.DiagnosticConfig{
			Reporting: mib.ReportingQuiet,
			FailAt:    mib.SeverityFatal,
		}),
	)
	require.NoError(t, err)

	c := &mibCorpus{
		mib:     m,
		modules: map[string]*mib.Module{},
		objects: map[string]map[string]*mib.Object{},
	}
	for _, mod := range m.Modules() {
		name := strings.ToLower(mod.Name())
		c.modules[name] = mod
		c.objects[name] = map[string]*mib.Object{}
		for _, obj := range mod.Objects() {
			c.objects[name][obj.OID().String()] = obj
		}
	}
	return c
}

// object resolves an OID in the row's MIB module first: vendors reuse an OID root across incompatible MIB versions
// (for example FREENAS-MIB and TRUENAS-MIB), and the row's module names the version the profile follows.
func (c *mibCorpus) object(module string, oid mib.OID) *mib.Object {
	if obj := c.objects[strings.ToLower(module)][oid.String()]; obj != nil {
		return obj
	}
	if node := c.mib.NodeByOID(oid); node != nil {
		return node.Object()
	}
	return nil
}

// nearestObjectAbove returns the closest object whose OID is a strict prefix of oid.
func (c *mibCorpus) nearestObjectAbove(module string, oid mib.OID) *mib.Object {
	for i := len(oid) - 1; i > 0; i-- {
		if obj := c.object(module, oid[:i]); obj != nil {
			return obj
		}
	}
	return nil
}

// check returns a problem for a wrong reference, or resolved=false when the corpus cannot verify it.
func (c *mibCorpus) check(ref profileOIDRef) (problem string, resolved bool) {
	oid, err := mib.ParseOID(strings.TrimPrefix(ref.oid, "."))
	if err != nil {
		return "", false
	}
	exact := c.object(ref.module, oid)
	above := c.nearestObjectAbove(ref.module, oid)

	var target *mib.Object
	if ref.column {
		switch {
		case exact != nil && exact.Kind() == mib.KindColumn:
			target = exact
		case exact != nil:
			return fmt.Sprintf("names the %s %s, not a column", exact.Kind(), exact.Name()), true
		case above != nil && isLeafObject(above):
			return fmt.Sprintf("is below the %s %s (doubled entry arc or extra arcs)", above.Kind(), above.Name()), true
		default:
			return "", false
		}
	} else {
		switch {
		case exact != nil:
			return fmt.Sprintf("names the %s %s itself; the instance suffix (.0) is missing", exact.Kind(), exact.Name()), true
		case above == nil:
			return "", c.mib.NodeByOID(oid) != nil // a named instance node such as sysUpTimeInstance
		case above.Kind() == mib.KindScalar && (len(oid) != len(above.OID())+1 || oid[len(oid)-1] != 0):
			return fmt.Sprintf("is not the .0 instance of the scalar %s", above.Name()), true
		case !isLeafObject(above):
			return "", false
		}
		target = above // a scalar .0 or a table cell read by GET
	}

	if other := c.otherObjectNamed(ref, target); other != nil {
		return fmt.Sprintf("its name is %s::%s at %s, but the OID is %s", other.Module().Name(), other.Name(),
			other.OID(), target.Name()), true
	}
	return "", true
}

// otherObjectNamed returns the object of the row's MIB module that the symbol name refers to, when that is not the
// object at the configured OID. Names keep only their last dot segment without leading underscores, so a profile
// prefix ("servertech.sentry4.") or the hidden-metric marker does not hide the MIB name.
func (c *mibCorpus) otherObjectNamed(ref profileOIDRef, target *mib.Object) *mib.Object {
	mod := c.modules[strings.ToLower(ref.module)]
	if mod == nil {
		return nil
	}
	name := ref.name
	if i := strings.LastIndex(name, "."); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimLeft(name, "_")
	if name == "" || name == target.Name() {
		return nil
	}
	other := mod.Object(name)
	if other == nil || other.OID().String() == target.OID().String() || !isPollable(other) {
		return nil
	}
	return other
}

// isPollable reports whether an object can be read by GET or walk; notification-only and index-only objects are not
// what a polled symbol name means.
func isPollable(obj *mib.Object) bool {
	return obj.Access() != mib.AccessNotAccessible && obj.Access() != mib.AccessAccessibleForNotify
}

func isLeafObject(obj *mib.Object) bool {
	return obj.Kind() == mib.KindColumn || obj.Kind() == mib.KindScalar
}
