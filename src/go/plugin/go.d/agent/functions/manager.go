// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/pkg/safewriter"
)

func NewManager() *Manager {
	return &Manager{
		Logger: logger.New().With(
			slog.String("component", "functions manager"),
		),
		api:              netdataapi.New(safewriter.Stdout),
		input:            stdinInput,
		mux:              &sync.Mutex{},
		FunctionRegistry: make(map[string]func(Function)),
	}
}

type Manager struct {
	*logger.Logger

	api *netdataapi.API

	input input

	mux              *sync.Mutex
	FunctionRegistry map[string]func(Function)
}

func (m *Manager) Run(ctx context.Context) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() { defer wg.Done(); m.run(ctx) }()

	wg.Wait()

	<-ctx.Done()
}

func (m *Manager) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case line, ok := <-m.input.lines():
			if !ok {
				return
			}

			var fn *Function
			var err error

			// FIXME:  if we are waiting for FUNCTION_PAYLOAD_END and a new FUNCTION* appears,
			// we need to discard the current one and switch to the new one
			switch {
			case strings.HasPrefix(line, "FUNCTION "):
				fn, err = parseFunction(line)
			case strings.HasPrefix(line, "FUNCTION_PAYLOAD "):
				fn, err = parseFunctionWithPayload(ctx, line, m.input)
			case line == "":
				continue
			default:
				m.Warningf("unexpected line: '%s'", line)
				continue
			}

			if err != nil {
				m.Warningf("parse function: %v ('%s')", err, line)
				continue
			}
			if fn == nil {
				continue
			}

			function, ok := m.lookupFunction(fn.Name)
			if !ok {
				m.Infof("skipping execution of '%s': unregistered function", fn.Name)
				m.respf(fn, 501, "unregistered function: %s", fn.Name)
				continue
			}
			if function == nil {
				m.Warningf("skipping execution of '%s': nil function registered", fn.Name)
				m.respf(fn, 501, "nil function: %s", fn.Name)
				continue
			}

			function(*fn)
		}
	}
}

func (m *Manager) lookupFunction(name string) (func(Function), bool) {
	m.mux.Lock()
	defer m.mux.Unlock()

	f, ok := m.FunctionRegistry[name]
	return f, ok
}

func (m *Manager) respf(fn *Function, code int, msgf string, a ...any) {
	bs, _ := json.Marshal(struct {
		Status  int    `json:"status"`
		Message string `json:"message"`
	}{
		Status:  code,
		Message: fmt.Sprintf(msgf, a...),
	})
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m.api.FUNCRESULT(fn.UID, "application/json", string(bs), strconv.Itoa(code), ts)
}
