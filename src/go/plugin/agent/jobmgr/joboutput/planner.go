// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type Planner struct {
	graph *dyncfg.Graph
}

type JobDeclaration struct {
	ID      string
	Module  string
	Name    string
	Payload []byte
}

func NewPlanner(graph *dyncfg.Graph) (*Planner, error) {
	if graph == nil {
		return nil, errors.New("job output: nil DynCfg graph")
	}
	return &Planner{graph: graph}, nil
}

func NewPlannerFromDeclarations(declarations []JobDeclaration) (*Planner, error) {
	configs := make([]dyncfg.GraphConfig, len(declarations))
	for index, declaration := range declarations {
		configs[index] = dyncfg.GraphConfig{
			ID: declaration.ID, Module: declaration.Module, Name: declaration.Name,
			Payload: append([]byte(nil), declaration.Payload...),
		}
	}
	graph, err := dyncfg.NewGraph(configs)
	if err != nil {
		return nil, err
	}
	return NewPlanner(graph)
}

func (planner *Planner) IDs() []string {
	return planner.graph.IDs()
}

func (planner *Planner) Plan(request jobmgr.Request) (jobmgr.WorkPlan, error) {
	route := request.Route
	args := request.Args
	if route != "config" || len(args) != 2 || args[1] != "get" {
		return jobmgr.RejectionPlan(lifecycle.ControlBadRequest), nil
	}
	config, ok := planner.graph.Lookup(args[0])
	if !ok {
		return jobmgr.RejectionPlan(lifecycle.ControlNotFound, "job:"+args[0]), nil
	}
	payload := []byte(config.Payload())
	return jobmgr.WorkPlan{
		Claims: []string{"job:" + config.ID},
		Work: lifecycle.FrameTaskWork(func(context.Context) (lifecycle.SealedResult, error) {
			return lifecycle.NewSealedResult(200, "application/json", payload)
		}),
	}, nil
}
