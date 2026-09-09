// SPDX-License-Identifier: GPL-3.0-or-later

package corpus

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
)

// ManifestCase describes one corpus case. It records what the case proves
// and, once a fix has landed, which PR delivered it.
//
// It deliberately records NO expected outcome. A contract either holds or
// it does not, and the only thing that can answer that is running it. A
// hardcoded "we know this one is broken" would make a broken engine report
// success, and the whole point of this corpus is to name what is broken.
type ManifestCase struct {
	Proves     string   `json:"proves"`
	Cloud      string   `json:"cloud,omitempty"`      // defaults to n/a
	FixedBy    string   `json:"fixed_by,omitempty"`   // PR or commit that fixed it
	Components []string `json:"components,omitempty"` // required independent test scopes; empty means one scope
}

//go:embed manifest.json
var manifestData []byte

type manifestRecord struct {
	Name string `json:"name"`
	ManifestCase
}

func loadManifest(data []byte) (map[string]ManifestCase, error) {
	var records []manifestRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, fmt.Errorf("decode query corpus manifest: %w", err)
	}

	cases := make(map[string]ManifestCase, len(records))
	for _, record := range records {
		if record.Name == "" {
			return nil, fmt.Errorf("query corpus manifest has an empty contract name")
		}
		if _, found := cases[record.Name]; found {
			return nil, fmt.Errorf("query corpus manifest repeats contract %q", record.Name)
		}
		cases[record.Name] = record.ManifestCase
	}

	if len(cases) == 0 {
		return nil, fmt.Errorf("query corpus manifest has no contracts")
	}

	return cases, nil
}

func mustLoadManifest() map[string]ManifestCase {
	cases, err := loadManifest(manifestData)
	if err != nil {
		panic(err)
	}
	return cases
}

var manifest = mustLoadManifest()

const defaultContractComponent = "contract"

type contractObservation struct {
	evaluated bool
	broken    bool
}

type contractLedger struct {
	mu      sync.Mutex
	results map[string]map[string]contractObservation
}

func newContractLedger() *contractLedger {
	return &contractLedger{results: make(map[string]map[string]contractObservation)}
}

func requiredContractComponents(mc ManifestCase) []string {
	if len(mc.Components) == 0 {
		return []string{defaultContractComponent}
	}
	return mc.Components
}

func validateContractComponent(name, component string) error {
	mc, ok := manifest[name]
	if !ok {
		return fmt.Errorf("case %q missing from manifest", name)
	}

	for _, required := range requiredContractComponents(mc) {
		if component == required {
			return nil
		}
	}

	return fmt.Errorf("case %q has no component %q", name, component)
}

func (l *contractLedger) record(name, component string, held, skipped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// A clean skip did not evaluate the contract. A test that failed before
	// it skipped still produced a real broken observation and must not vanish.
	if skipped && held {
		return
	}

	components := l.results[name]
	if components == nil {
		components = make(map[string]contractObservation)
		l.results[name] = components
	}

	observation := components[component]
	observation.evaluated = true
	if !held {
		observation.broken = true
	}
	components[component] = observation
}

func (l *contractLedger) register(name, component string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	components := l.results[name]
	if components == nil {
		components = make(map[string]contractObservation)
		l.results[name] = components
	}
	if _, ok := components[component]; !ok {
		components[component] = contractObservation{}
	}
}

// trackContract records the result of an ordinary Go test as one corpus
// contract. Register it before any assertion so Fatal and Skip are visible.
func trackContract(t *testing.T, name string) {
	t.Helper()
	trackContractComponent(t, name, defaultContractComponent)
}

// trackContractComponent records one required scope of a contract that is
// proven by more than one independent test.
func trackContractComponent(t *testing.T, name, component string) {
	t.Helper()

	if err := validateContractComponent(name, component); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		contractResults.record(name, component, !t.Failed(), t.Skipped())
	})
}

// registerContract reserves a computed contract verdict before shared work.
// The later assertContract call evaluates it; an earlier Fatal or Skip leaves
// the reservation incomplete instead of allowing a default-true verdict.
func registerContract(t *testing.T, name string) {
	t.Helper()
	registerContractComponent(t, name, defaultContractComponent)
}

func registerContractComponent(t *testing.T, name, component string) {
	t.Helper()

	if err := validateContractComponent(name, component); err != nil {
		t.Fatal(err)
	}

	contractResults.register(name, component)
}

// assertContract records the verdict for one corpus case.
//
// A contract either holds or it does not. A broken one fails here - on
// master, on a feature branch, whether or not anyone already knew about
// it. There is no "known broken, and therefore fine": that state makes a
// broken query engine report success, and this corpus exists to name what
// is broken, not to keep a list of exceptions.
//
// Every break is also collected for the end-of-run summary, so a run tells
// you the whole set at once instead of one line per test.
func assertContract(t *testing.T, name string, held bool) {
	t.Helper()

	if err := validateContractComponent(name, defaultContractComponent); err != nil {
		t.Fatal(err)
	}

	contractResults.record(name, defaultContractComponent, held, false)
	if held {
		return
	}

	t.Errorf("BROKEN %s: %s", name, manifest[name].Proves)
}

var contractResults = newContractLedger()

type contractRunSummary struct {
	evaluated  int
	broken     []string
	incomplete []string
}

func (l *contractLedger) summarize(cases map[string]ManifestCase) contractRunSummary {
	l.mu.Lock()
	defer l.mu.Unlock()

	var summary contractRunSummary
	for name, mc := range cases {
		complete := true
		broken := false
		for _, component := range requiredContractComponents(mc) {
			observation, ok := l.results[name][component]
			if !ok || !observation.evaluated {
				complete = false
				if len(mc.Components) == 0 {
					summary.incomplete = append(summary.incomplete, name)
				} else {
					summary.incomplete = append(summary.incomplete, name+"/"+component)
				}
			}
			if observation.broken {
				broken = true
			}
		}
		if complete {
			summary.evaluated++
		}
		if broken {
			summary.broken = append(summary.broken, name)
		}
	}

	sort.Strings(summary.broken)
	sort.Strings(summary.incomplete)
	return summary
}

// contractSummary is printed once, after every test has run. complete is true
// only when every manifest contract and each of its required scopes ran.
func contractSummary(includeIncompleteDetails bool) (report string, complete bool) {
	summary := contractResults.summarize(manifest)
	return formatContractSummary(summary, len(manifest), includeIncompleteDetails)
}

func formatContractSummary(summary contractRunSummary, total int, includeIncompleteDetails bool) (report string, complete bool) {
	complete = len(summary.incomplete) == 0

	var b strings.Builder
	if complete && len(summary.broken) == 0 {
		fmt.Fprintf(&b, "query contract corpus: all %d contracts hold\n", total)
		return b.String(), true
	}

	if !complete {
		fmt.Fprintf(&b, "\nquery contract corpus: %d of %d contracts fully evaluated; %d required scope(s) did not run\n",
			summary.evaluated, total, len(summary.incomplete))
	}

	if len(summary.broken) > 0 {
		if complete {
			fmt.Fprintf(&b, "\nquery contract corpus: %d of %d contracts BROKEN\n",
				len(summary.broken), total)
		} else {
			fmt.Fprintf(&b, "query contract corpus: %d contract(s) reported BROKEN\n",
				len(summary.broken))
		}
		for _, name := range summary.broken {
			fmt.Fprintf(&b, "  BROKEN  %s\n", name)
		}
		fmt.Fprintln(&b, "\nEach one is a defect in the query engine, not a test to adjust.")
	} else {
		fmt.Fprintln(&b, "query contract corpus: no evaluated contract reported broken")
	}

	if !complete && includeIncompleteDetails {
		fmt.Fprintln(&b, "\nRequired contract scopes not run:")
		for _, name := range summary.incomplete {
			fmt.Fprintf(&b, "  NOT RUN  %s\n", name)
		}
	}

	return b.String(), complete
}
