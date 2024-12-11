package consul

import (
	"math"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/metrix"
)

const (
	// https://developer.hashicorp.com/consul/api-docs/coordinate#read-lan-coordinates-for-all-nodes
	urlPathCoordinateNodes = "/v1/coordinate/nodes"
)

type nodeCoordinates struct {
	Node  string
	Coord struct {
		Vec        []float64
		Error      float64
		Adjustment float64
		Height     float64
	}
}

func (c *Collector) collectNetworkRTT(mx map[string]int64) error {
	req, err := c.createRequest(urlPathCoordinateNodes)
	if err != nil {
		return err
	}

	var coords []nodeCoordinates

	if err := c.client().RequestJSON(req, &coords); err != nil {
		return err
	}

	var thisNode nodeCoordinates
	var ok bool

	coords, thisNode, ok = removeNodeCoordinates(coords, c.cfg.Config.NodeName)
	if !ok || len(coords) == 0 {
		return nil
	}

	sum := metrix.NewSummary()
	for _, v := range coords {
		d := calcDistance(thisNode, v)
		sum.Observe(d.Seconds())
	}
	sum.WriteTo(mx, "network_lan_rtt", 1e9, 1)

	return nil
}

func calcDistance(a, b nodeCoordinates) time.Duration {
	// https://developer.hashicorp.com/consul/docs/architecture/coordinates#working-with-coordinates
	sum := 0.0
	for i := 0; i < len(a.Coord.Vec); i++ {
		diff := a.Coord.Vec[i] - b.Coord.Vec[i]
		sum += diff * diff
	}

	rtt := math.Sqrt(sum) + a.Coord.Height + b.Coord.Height

	adjusted := rtt + a.Coord.Adjustment + b.Coord.Adjustment
	if adjusted > 0.0 {
		rtt = adjusted
	}

	return time.Duration(rtt * 1e9) // nanoseconds
}

func removeNodeCoordinates(coords []nodeCoordinates, node string) ([]nodeCoordinates, nodeCoordinates, bool) {
	for i, v := range coords {
		if v.Node == node {
			return append(coords[:i], coords[i+1:]...), v, true
		}
	}
	return coords, nodeCoordinates{}, false
}
