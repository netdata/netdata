// SPDX-License-Identifier: GPL-3.0-or-later

package dockerfunc

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	typesContainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-units"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
)

const (
	containersMethodID   = "container-ls"
	containersMethodHelp = "List Docker containers (equivalent to docker ps -a)."
)

const (
	colContainerID = iota
	colImage
	colCommand
	colCreated
	colStatus
	colState
	colPorts
	colNames
	colContainerIDFull
	colCreatedUnix
)

const (
	containersColID          = "container_id"
	containersColImage       = "image"
	containersColCommand     = "command"
	containersColCreated     = "created"
	containersColStatus      = "status"
	containersColState       = "state"
	containersColPorts       = "ports"
	containersColNames       = "names"
	containersColIDFull      = "container_id_full"
	containersColCreatedUnix = "created_unix"
)

func containersMethodConfig() funcapi.MethodConfig {
	return funcapi.MethodConfig{
		ID:          containersMethodID,
		Name:        "Containers",
		UpdateEvery: 10,
		Help:        containersMethodHelp,
	}
}

type funcContainers struct {
	router *router
}

func newFuncContainers(r *router) *funcContainers {
	return &funcContainers{router: r}
}

// Compile-time interface check.
var _ funcapi.MethodHandler = (*funcContainers)(nil)

func (f *funcContainers) MethodParams(_ context.Context, method string) ([]funcapi.ParamConfig, error) {
	if method != containersMethodID {
		return nil, fmt.Errorf("unknown method: %s", method)
	}
	return nil, nil
}

func (f *funcContainers) Handle(ctx context.Context, method string, params funcapi.ResolvedParams) *funcapi.FunctionResponse {
	if method != containersMethodID {
		return funcapi.NotFoundResponse(method)
	}

	client, err := f.router.deps.DockerClient()
	if err != nil {
		return funcapi.UnavailableResponse("collector is still initializing, please retry in a few seconds")
	}

	containers, err := client.ContainerList(ctx, typesContainer.ListOptions{All: true})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return funcapi.ErrorResponse(504, "query timed out")
		}
		return funcapi.ErrorResponse(500, "failed to list containers: %v", err)
	}

	sortContainers(containers)

	now := time.Now()
	rows := make([][]any, 0, len(containers))
	for _, cntr := range containers {
		rows = append(rows, buildContainerRow(cntr, now))
	}

	return &funcapi.FunctionResponse{
		Status:            200,
		Help:              containersMethodHelp,
		Columns:           buildContainerColumns(),
		Data:              rows,
		DefaultSortColumn: containersColCreatedUnix,
	}
}

func (f *funcContainers) Cleanup(ctx context.Context) {}

func sortContainers(containers []typesContainer.Summary) {
	sort.SliceStable(containers, func(i, j int) bool {
		if containers[i].Created != containers[j].Created {
			return containers[i].Created > containers[j].Created
		}
		return containers[i].ID < containers[j].ID
	})
}

func buildContainerRow(cntr typesContainer.Summary, now time.Time) []any {
	row := make([]any, colCreatedUnix+1)
	row[colContainerID] = shortContainerID(cntr.ID)
	row[colImage] = cntr.Image
	row[colCommand] = strings.TrimSpace(cntr.Command)
	row[colCreated] = formatCreated(cntr.Created, now)
	row[colStatus] = formatStatus(cntr)
	row[colState] = strings.TrimSpace(cntr.State)
	row[colPorts] = formatPorts(cntr.Ports)
	row[colNames] = formatContainerNames(cntr.Names)
	row[colContainerIDFull] = cntr.ID
	row[colCreatedUnix] = cntr.Created
	return row
}

func buildContainerColumns() map[string]any {
	return map[string]any{
		containersColID: funcapi.Column{
			Index:         colContainerID,
			Name:          "CONTAINER ID",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Sticky:        true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColImage: funcapi.Column{
			Index:         colImage,
			Name:          "IMAGE",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
			Wrap:          true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColCommand: funcapi.Column{
			Index:         colCommand,
			Name:          "COMMAND",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterNone,
			Visible:       true,
			Wrap:          true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColCreated: funcapi.Column{
			Index:         colCreated,
			Name:          "CREATED",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortDescending,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColStatus: funcapi.Column{
			Index:         colStatus,
			Name:          "STATUS",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterNone,
			Visible:       true,
			Wrap:          true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColState: funcapi.Column{
			Index:         colState,
			Name:          "State (Raw)",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       false,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColPorts: funcapi.Column{
			Index:         colPorts,
			Name:          "PORTS",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterNone,
			Visible:       true,
			Wrap:          true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColNames: funcapi.Column{
			Index:         colNames,
			Name:          "NAMES",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       true,
			Sticky:        true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColIDFull: funcapi.Column{
			Index:         colContainerIDFull,
			Name:          "Container ID (Full)",
			Type:          funcapi.FieldTypeString,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortAscending,
			Sortable:      false,
			Summary:       funcapi.FieldSummaryCount,
			Filter:        funcapi.FieldFilterMultiselect,
			Visible:       false,
			UniqueKey:     true,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformText},
		}.BuildColumn(),
		containersColCreatedUnix: funcapi.Column{
			Index:         colCreatedUnix,
			Name:          "Created (Unix)",
			Type:          funcapi.FieldTypeInteger,
			Visualization: funcapi.FieldVisualValue,
			Sort:          funcapi.FieldSortDescending,
			Sortable:      true,
			Summary:       funcapi.FieldSummaryMax,
			Filter:        funcapi.FieldFilterNone,
			Visible:       false,
			ValueOptions:  funcapi.ValueOptions{Transform: funcapi.FieldTransformNumber},
		}.BuildColumn(),
	}
}

func shortContainerID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func formatContainerNames(names []string) string {
	if len(names) == 0 {
		return ""
	}

	clean := make([]string, 0, len(names))
	for _, name := range names {
		v := strings.TrimPrefix(strings.TrimSpace(name), "/")
		if v != "" {
			clean = append(clean, v)
		}
	}
	return strings.Join(clean, ", ")
}

func formatCreated(created int64, now time.Time) string {
	if created <= 0 {
		return ""
	}
	createdAt := time.Unix(created, 0)
	if now.Before(createdAt) {
		return "0 seconds ago"
	}
	return units.HumanDuration(now.Sub(createdAt)) + " ago"
}

func formatStatus(cntr typesContainer.Summary) string {
	if cntr.Status != "" {
		return cntr.Status
	}

	switch cntr.State {
	case "running":
		return "Up"
	case "paused":
		return "Paused"
	case "restarting":
		return "Restarting"
	case "removing":
		return "Removing"
	case "exited":
		return "Exited"
	case "dead":
		return "Dead"
	case "created":
		return "Created"
	default:
		return cntr.State
	}
}

func formatPorts(ports []typesContainer.Port) string {
	if len(ports) == 0 {
		return ""
	}

	clone := append([]types.Port(nil), ports...)
	sort.Slice(clone, func(i, j int) bool {
		if clone[i].PrivatePort != clone[j].PrivatePort {
			return clone[i].PrivatePort < clone[j].PrivatePort
		}
		if clone[i].PublicPort != clone[j].PublicPort {
			return clone[i].PublicPort < clone[j].PublicPort
		}
		if clone[i].IP != clone[j].IP {
			return clone[i].IP < clone[j].IP
		}
		return clone[i].Type < clone[j].Type
	})

	out := make([]string, 0, len(clone))
	for _, p := range clone {
		v := formatPort(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return strings.Join(out, ", ")
}

func formatPort(p typesContainer.Port) string {
	proto := p.Type
	if proto == "" {
		proto = "tcp"
	}

	switch {
	case p.PublicPort > 0 && p.IP != "":
		return fmt.Sprintf("%s:%d->%d/%s", p.IP, p.PublicPort, p.PrivatePort, proto)
	case p.PublicPort > 0:
		return fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, proto)
	case p.PrivatePort > 0:
		return fmt.Sprintf("%d/%s", p.PrivatePort, proto)
	default:
		return ""
	}
}
