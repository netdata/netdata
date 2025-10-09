package as400

import "context"

const (
	networkInterfaceLimit = 50
	httpServerLimit       = 200
	messageQueueLimit     = 200
	outputQueueLimit      = 200
)

type cardinalityGuard struct {
	max       int
	checked   bool
	allowed   bool
	lastCount int
}

func (g *cardinalityGuard) Configure(max int) {
	if g.max != max {
		g.max = max
		g.checked = false
		g.allowed = true
		g.lastCount = 0
	}
}

func (g *cardinalityGuard) Allow(ctx context.Context, counter func(context.Context) (int, error)) (bool, int, error) {
	if g.max <= 0 {
		g.checked = true
		g.allowed = true
		g.lastCount = 0
		return true, 0, nil
	}
	if g.checked {
		return g.allowed, g.lastCount, nil
	}
	count, err := counter(ctx)
	if err != nil {
		return false, 0, err
	}
	g.checked = true
	g.lastCount = count
	g.allowed = count <= g.max
	return g.allowed, count, nil
}

func (g *cardinalityGuard) Exceeded() bool {
	return g.checked && !g.allowed
}

func (g *cardinalityGuard) LastCount() int {
	return g.lastCount
}

func (g *cardinalityGuard) Max() int {
	return g.max
}
