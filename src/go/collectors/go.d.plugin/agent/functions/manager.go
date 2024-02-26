// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/go.d.plugin/logger"

	"github.com/mattn/go-isatty"
	"github.com/muesli/cancelreader"
)

var isTerminal = isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsTerminal(os.Stdin.Fd())

func NewManager() *Manager {
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "functions manager"),
		),
		Input:            os.Stdin,
		mux:              &sync.Mutex{},
		FunctionRegistry: make(map[string]func(Function)),
	}
}

type Manager struct {
	*logger.Logger

	Input            io.Reader
	mux              *sync.Mutex
	FunctionRegistry map[string]func(Function)
}

func (m *Manager) Register(name string, fn func(Function)) {
	if fn == nil {
		m.Warningf("not registering '%s': nil function", name)
		return
	}
	m.addFunction(name, fn)
}

func (m *Manager) Run(ctx context.Context) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	if !isTerminal {
		var wg sync.WaitGroup

		r, err := cancelreader.NewReader(m.Input)
		if err != nil {
			m.Errorf("fail to create cancel reader: %v", err)
			return
		}

		go func() { <-ctx.Done(); r.Cancel() }()

		wg.Add(1)
		go func() { defer wg.Done(); m.run(r) }()

		wg.Wait()
		_ = r.Close()
	}

	<-ctx.Done()
}

func (m *Manager) run(r io.Reader) {
	sc := bufio.NewScanner(r)

	for sc.Scan() {
		text := sc.Text()

		var fn *Function
		var err error

		// FIXME:  if we are waiting for FUNCTION_PAYLOAD_END and a new FUNCTION* appears,
		// we need to discard the current one and switch to the new one
		switch {
		case strings.HasPrefix(text, "FUNCTION "):
			fn, err = parseFunction(text)
		case strings.HasPrefix(text, "FUNCTION_PAYLOAD "):
			fn, err = parseFunctionWithPayload(text, sc)
		case text == "":
			continue
		default:
			m.Warningf("unexpected line: '%s'", text)
			continue
		}

		if err != nil {
			m.Warningf("parse function: %v ('%s')", err, text)
			continue
		}

		function, ok := m.lookupFunction(fn.Name)
		if !ok {
			m.Infof("skipping execution of '%s': unregistered function", fn.Name)
			continue
		}
		if function == nil {
			m.Warningf("skipping execution of '%s': nil function registered", fn.Name)
			continue
		}

		m.Debugf("executing function: '%s'", fn.String())
		function(*fn)
	}
}

func (m *Manager) addFunction(name string, fn func(Function)) {
	m.mux.Lock()
	defer m.mux.Unlock()

	if _, ok := m.FunctionRegistry[name]; !ok {
		m.Debugf("registering function '%s'", name)
	} else {
		m.Warningf("re-registering function '%s'", name)
	}
	m.FunctionRegistry[name] = fn
}

func (m *Manager) lookupFunction(name string) (func(Function), bool) {
	m.mux.Lock()
	defer m.mux.Unlock()

	f, ok := m.FunctionRegistry[name]
	return f, ok
}
