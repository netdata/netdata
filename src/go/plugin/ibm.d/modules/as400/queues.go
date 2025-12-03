//go:build cgo
// +build cgo

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

type activeJobTarget struct {
	Number string
	User   string
	Name   string
}

func (t activeJobTarget) ID() string {
	return t.Number + "/" + t.User + "/" + t.Name
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

func parseActiveJobTargets(entries []string) ([]activeJobTarget, error) {
	var targets []activeJobTarget
	seen := make(map[string]struct{})

	for _, raw := range entries {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}

		parts := strings.Split(trimmed, "/")
		if len(parts) != 3 {
			return nil, fmt.Errorf("invalid active job identifier %q (expected JOB_NUMBER/USER/JOB_NAME)", raw)
		}

		number := strings.TrimSpace(parts[0])
		user := strings.ToUpper(strings.TrimSpace(parts[1]))
		name := strings.ToUpper(strings.TrimSpace(parts[2]))

		if !validJobNumber(number) {
			return nil, fmt.Errorf("invalid active job identifier %q (job number must be six digits)", raw)
		}
		if !validObjectName(user) || !validObjectName(name) {
			return nil, fmt.Errorf("invalid active job identifier %q (only A-Z, 0-9, _, $, #, @ allowed)", raw)
		}

		key := number + "/" + user + "/" + name
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, activeJobTarget{
			Number: number,
			User:   user,
			Name:   name,
		})
	}

	return targets, nil
}

func (c *Collector) configureTargets() error {
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

	jobTargets, err := parseActiveJobTargets(c.ActiveJobs)
	if err != nil {
		return fmt.Errorf("active_jobs configuration error: %w", err)
	}
	c.activeJobTargets = jobTargets

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
	if len(c.activeJobTargets) == 0 {
		c.Infof("active job metrics disabled: no jobs configured")
	} else {
		c.Infof("active job metrics enabled for %d job(s): %s", len(c.activeJobTargets), activeJobTargetList(c.activeJobTargets))
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

func activeJobTargetList(targets []activeJobTarget) string {
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

func validJobNumber(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
