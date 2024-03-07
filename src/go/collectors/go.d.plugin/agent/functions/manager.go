// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/netdataapi"
	"github.com/netdata/netdata/go/go.d.plugin/agent/safewriter"
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
		api:              netdataapi.New(safewriter.Stdout),
		mux:              &sync.Mutex{},
		FunctionRegistry: make(map[string]func(Function)),
	}
}

type Manager struct {
	*logger.Logger

	Input            io.Reader
	api              *netdataapi.API
	mux              *sync.Mutex
	FunctionRegistry map[string]func(Function)
}

func (m *Manager) Run(ctx context.Context) {
	m.Info("instance is started")
	defer func() { m.Info("instance is stopped") }()

	if !isTerminal {
		r, err := cancelreader.NewReader(m.Input)
		if err != nil {
			m.Errorf("fail to create cancel reader: %v", err)
			return
		}

		go func() { <-ctx.Done(); r.Cancel() }()

		var wg sync.WaitGroup

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
