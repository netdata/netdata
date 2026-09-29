// SPDX-License-Identifier: GPL-3.0-or-later

package nativefunc

import (
	"errors"
	"regexp"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/native/internal/strictjson"
)

const (
	defaultUpdateEvery  = 10
	defaultResponseType = "table"
	// jobSelector is the framework's job selection parameter.
	jobSelector = "__job"
)

var (
	reMethodID = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	reParamID  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
)

// Definition is startup metadata, independent of any running script or job.
type Definition struct {
	ID             string      `yaml:"id"              json:"id"`
	Name           string      `yaml:"name"            json:"name"`
	Help           string      `yaml:"help"            json:"help"`
	UpdateEvery    int         `yaml:"update_every"    json:"update_every"`
	ResponseType   string      `yaml:"response_type"   json:"response_type"`
	HasHistory     bool        `yaml:"has_history"     json:"has_history"`
	AcceptedParams []string    `yaml:"accepted_params" json:"accepted_params"`
	RequiredParams []Parameter `yaml:"required_params" json:"required_params"`
}

type Parameter struct {
	ID         string   `yaml:"id"                    json:"id"`
	Name       string   `yaml:"name"                  json:"name"`
	Help       string   `yaml:"help,omitempty"        json:"help,omitempty"`
	Type       string   `yaml:"type"                  json:"type"`
	Options    []Option `yaml:"options"               json:"options"`
	UniqueView bool     `yaml:"unique_view,omitempty" json:"unique_view,omitempty"`
}

type Option struct {
	ID       string `yaml:"id"                        json:"id"`
	Name     string `yaml:"name"                      json:"name"`
	Default  bool   `yaml:"defaultSelected,omitempty" json:"defaultSelected,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"        json:"disabled,omitempty"`
}

var (
	parameterFields = []string{"id", "name", "help", "type", "unique_view"}
	optionFields    = []string{"id", "name", "defaultSelected", "disabled"}
)

// DefinitionShape is the strict JSON shape of a Definition in package
// metadata, where null selects a default.
func DefinitionShape() *strictjson.Shape {
	return strictjson.Optional(strictjson.Fields{
		"required_params": parameterShape(strictjson.Optional),
	}, "id", "name", "help", "update_every", "response_type", "has_history", "accepted_params")
}

func parameterShape(object func(strictjson.Fields, ...string) *strictjson.Shape) *strictjson.Shape {
	return object(strictjson.Fields{
		"options": object(nil, optionFields...),
	}, parameterFields...)
}

// Methods validates declared Functions and returns their raw-request method
// configurations. Info requests are managed by the framework.
func Methods(definitions []Definition) ([]funcapi.FunctionConfig, error) {
	methods := make([]funcapi.FunctionConfig, 0, len(definitions))
	seen := map[string]bool{}
	for _, d := range definitions {
		if !reMethodID.MatchString(d.ID) || seen[d.ID] || strings.TrimSpace(d.Name) == "" ||
			strings.TrimSpace(d.Help) == "" {
			return nil, errors.New("functions require unique valid IDs, names and help")
		}
		seen[d.ID] = true
		if d.UpdateEvery < 0 {
			return nil, errors.New("function update_every must not be negative")
		}
		if d.UpdateEvery == 0 {
			d.UpdateEvery = defaultUpdateEvery
		}
		if d.ResponseType == "" {
			d.ResponseType = defaultResponseType
		}
		params, err := paramConfigs(d.RequiredParams)
		if err != nil {
			return nil, err
		}
		accepted := map[string]bool{}
		for _, key := range d.AcceptedParams {
			if !reParamID.MatchString(key) || key == jobSelector || accepted[key] {
				return nil, errors.New("invalid or duplicate accepted Function parameter")
			}
			accepted[key] = true
		}
		methods = append(methods, funcapi.FunctionConfig{
			ID:             d.ID,
			Name:           d.Name,
			Help:           d.Help,
			UpdateEvery:    d.UpdateEvery,
			ResponseType:   d.ResponseType,
			RawRequest:     true,
			ManagedInfo:    true,
			HasHistory:     d.HasHistory,
			AcceptedParams: d.AcceptedParams,
			RequiredParams: params,
		})
	}
	return methods, nil
}

func paramConfigs(params []Parameter) ([]funcapi.ParamConfig, error) {
	result := make([]funcapi.ParamConfig, 0, len(params))
	seen := map[string]bool{}
	for _, p := range params {
		if !reParamID.MatchString(p.ID) || p.ID == jobSelector || seen[p.ID] || strings.TrimSpace(p.Name) == "" {
			return nil, errors.New("invalid or duplicate Function parameter")
		}
		seen[p.ID] = true
		selection := funcapi.ParamSelect
		switch p.Type {
		case "", "select":
		case "multiselect":
			selection = funcapi.ParamMultiSelect
		default:
			return nil, errors.New("invalid Function parameter selection type")
		}
		options := make([]funcapi.ParamOption, 0, len(p.Options))
		ids := map[string]bool{}
		for _, o := range p.Options {
			if o.ID == "" || o.Name == "" || ids[o.ID] {
				return nil, errors.New("invalid or duplicate Function parameter option")
			}
			ids[o.ID] = true
			options = append(options, funcapi.ParamOption{
				ID:       o.ID,
				Name:     o.Name,
				Default:  o.Default,
				Disabled: o.Disabled,
			})
		}
		result = append(result, funcapi.ParamConfig{
			ID:         p.ID,
			Name:       p.Name,
			Help:       p.Help,
			Selection:  selection,
			Options:    options,
			UniqueView: p.UniqueView,
		})
	}
	return result, nil
}
