package as400

import (
	"fmt"
	"strings"
)

type queueTarget struct {
	Library string
	Name    string
}

func (t queueTarget) ID() string {
	return t.Library + "/" + t.Name
}

func parseQueueTargets(entries []string) ([]queueTarget, error) {
	var targets []queueTarget
	seen := make(map[string]struct{})

	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "/", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid queue identifier %q (expected LIBRARY/QUEUE)", raw)
		}
		lib := strings.ToUpper(strings.TrimSpace(parts[0]))
		name := strings.ToUpper(strings.TrimSpace(parts[1]))
		if lib == "" || name == "" {
			return nil, fmt.Errorf("invalid queue identifier %q (library and queue must be non-empty)", raw)
		}
		if !validObjectName(lib) || !validObjectName(name) {
			return nil, fmt.Errorf("invalid queue identifier %q (only A-Z, 0-9, _, $, #, @ allowed)", raw)
		}
		key := lib + "/" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, queueTarget{
			Library: lib,
			Name:    name,
		})
	}

	return targets, nil
}

func (c *Collector) configureQueueTargets() error {
	targets, err := parseQueueTargets(c.MessageQueues)
	if err != nil {
		return fmt.Errorf("message_queues configuration error: %w", err)
	}
	c.messageQueueTargets = targets

	targets, err = parseQueueTargets(c.JobQueues)
	if err != nil {
		return fmt.Errorf("job_queues configuration error: %w", err)
	}
	c.jobQueueTargets = targets

	targets, err = parseQueueTargets(c.OutputQueues)
	if err != nil {
		return fmt.Errorf("output_queues configuration error: %w", err)
	}
	c.outputQueueTargets = targets

	if len(c.messageQueueTargets) == 0 {
		c.Infof("message queue metrics disabled: no queues configured")
	} else {
		c.Infof("message queue metrics enabled for %d queue(s): %s", len(c.messageQueueTargets), queueTargetList(c.messageQueueTargets))
	}

	if len(c.jobQueueTargets) == 0 {
		c.Infof("job queue metrics disabled: no queues configured")
	} else {
		c.Infof("job queue metrics enabled for %d queue(s): %s", len(c.jobQueueTargets), queueTargetList(c.jobQueueTargets))
	}

	if len(c.outputQueueTargets) == 0 {
		c.Infof("output queue metrics disabled: no queues configured")
	} else {
		c.Infof("output queue metrics enabled for %d queue(s): %s", len(c.outputQueueTargets), queueTargetList(c.outputQueueTargets))
	}

	return nil
}

func queueTargetList(targets []queueTarget) string {
	ids := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.ID())
	}
	return strings.Join(ids, ", ")
}

func validObjectName(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'A' && r <= 'Z':
			continue
		case r >= '0' && r <= '9':
			continue
		case r == '_' || r == '$' || r == '#' || r == '@':
			continue
		default:
			return false
		}
	}
	return true
}
