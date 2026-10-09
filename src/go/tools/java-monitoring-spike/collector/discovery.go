// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	PID                          int
	Start, UID, GID, Application string
}

func (t Target) key() string                  { return fmt.Sprintf("%d:%s", t.PID, t.Start) }
func (c *Collector) instance(t Target) string { return c.runtime.RunID + "-" + t.key() }

type targetState struct {
	Target
	Status, Detail, Runtime string
	LastSeen                time.Time
}

// The lab marker and private PID namespace are checked before Run. This selector
// deliberately matches only the owned fixture, not arbitrary Java applications.
func discover(root string) ([]Target, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var found []Target
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		t, err := inspect(filepath.Join(root, entry.Name()), pid)
		if err == nil && t != nil {
			found = append(found, *t)
		}
	}
	return found, nil
}

func inspect(proc string, pid int) (*Target, error) {
	exe, err := os.Readlink(filepath.Join(proc, "exe"))
	if err != nil || filepath.Base(exe) != "java" {
		return nil, err
	}
	cmd, err := os.ReadFile(filepath.Join(proc, "cmdline"))
	if err != nil {
		return nil, err
	}
	argv := strings.Split(string(cmd), "\x00")
	fixture := false
	application := ""
	for i, arg := range argv {
		if arg == "-jar" && i+1 < len(argv) && argv[i+1] == "/app/app.jar" {
			fixture = true
		}
		if strings.HasPrefix(arg, "--spring.application.name=") {
			application = strings.TrimPrefix(arg, "--spring.application.name=")
		}
	}
	if !fixture {
		return nil, nil
	}
	stat, err := os.ReadFile(filepath.Join(proc, "stat"))
	if err != nil {
		return nil, err
	}
	end := strings.LastIndex(string(stat), ") ")
	if end < 0 {
		return nil, fmt.Errorf("invalid process stat")
	}
	fields := strings.Fields(string(stat)[end+2:])
	if len(fields) <= 19 {
		return nil, fmt.Errorf("short process stat")
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return nil, err
	}
	t := &Target{PID: pid, Start: fields[19], Application: application}
	status, err := os.ReadFile(filepath.Join(proc, "status"))
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(status), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		if f[0] == "Uid:" {
			t.UID = f[2]
		}
		if f[0] == "Gid:" {
			t.GID = f[2]
		}
	}
	for _, id := range []string{t.UID, t.GID} {
		if _, err := strconv.ParseUint(id, 10, 32); err != nil {
			return nil, fmt.Errorf("missing or invalid credentials")
		}
	}
	if t.Application == "" {
		t.Application = "java-app-" + t.UID
	}
	// Scout's agent argument grammar uses semicolon-separated key/value pairs.
	if strings.ContainsAny(t.Application, ";,=\r\n") {
		return nil, fmt.Errorf("unsupported fixture application name")
	}
	return t, nil
}

type attempt struct{ Key, Status, Detail string }

func readAttempts(file *os.File) (map[string]attempt, error) {
	known := make(map[string]attempt)
	scan := bufio.NewScanner(file)
	for scan.Scan() {
		var a attempt
		if err := json.Unmarshal(scan.Bytes(), &a); err != nil {
			return nil, fmt.Errorf("attachment journal: %w", err)
		}
		if a.Key == "" || (a.Status != "Unknown" && a.Status != "Attached" && a.Status != "Blocked") {
			return nil, fmt.Errorf("invalid attachment journal record")
		}
		known[a.Key] = a
	}
	return known, scan.Err()
}

func saveAttempt(file *os.File, a attempt) error {
	if err := json.NewEncoder(file).Encode(a); err != nil {
		return err
	}
	return file.Sync()
}

func (c *Collector) scan(ctx context.Context, file *os.File, known map[string]attempt) error {
	found, err := discover("/proc")
	if err != nil {
		return err
	}
	live := make(map[string]bool)
	for _, t := range found {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		key := t.key()
		live[key] = true
		state := targetState{Target: t}
		if c.excluded(t.Application) {
			state.Status, state.Detail = "Excluded", "Excluded by application name; loaded instrumentation requires an application restart to unload"
			c.ingress.Remove(c.instance(t))
		} else {
			c.ingress.Admit(c.instance(t), t.Application)
			a, exists := known[key]
			if !exists {
				a = attempt{key, "Unknown", "Attachment recorded before launch; outcome unknown until acknowledged"}
				if err := saveAttempt(file, a); err != nil {
					return err
				}
				known[key] = a
				c.setTarget(key, targetState{Target: t, Status: "Attaching", Detail: a.Detail})
				a.Status, a.Detail = c.attach(ctx, t)
				if err := saveAttempt(file, a); err != nil {
					return err
				}
				known[key] = a
			}
			state.Status, state.Detail = a.Status, a.Detail
		}
		c.setTarget(key, state)
	}
	c.mu.Lock()
	var removed []Target
	for key, state := range c.targets {
		if !live[key] {
			removed = append(removed, state.Target)
			delete(c.targets, key)
		}
	}
	c.mu.Unlock()
	for _, t := range removed {
		c.ingress.Remove(c.instance(t))
	}
	return nil
}

func (c *Collector) setTarget(key string, state targetState) {
	c.mu.Lock()
	defer c.mu.Unlock()
	old := c.targets[key]
	state.LastSeen, state.Runtime = old.LastSeen, old.Runtime
	c.targets[key] = state
}

func (c *Collector) attach(ctx context.Context, t Target) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	options := []string{
		"otel.service.name=" + t.Application,
		"otel.resource.attributes=service.instance.id=" + c.instance(t),
		"otel.exporter.otlp.endpoint=" + c.runtime.Endpoint,
		"otel.exporter.otlp.protocol=http/protobuf", "otel.traces.exporter=none", "otel.logs.exporter=none",
		"otel.metric.export.interval=1000", "otel.exporter.otlp.metrics.temporality.preference=cumulative",
		"otel.exporter.otlp.metrics.default.histogram.aggregation=explicit_bucket_histogram",
		"otel.javaagent.extensions=/lab/hikari-extension.jar", "otel.instrumentation.hikaricp.enabled=false",
		"otel.instrumentation.common.default-enabled=false",
	}
	for _, name := range []string{"runtime-telemetry", "servlet", "tomcat", "spring-webmvc", "netdata-spike-hikari"} {
		options = append(options, "otel.instrumentation."+name+".enabled=true")
	}
	cmd := exec.CommandContext(ctx, "setpriv", "--reuid="+t.UID, "--regid="+t.GID,
		"--clear-groups", "--inh-caps=-all", "--ambient-caps=-all", "--no-new-privs",
		filepath.Join(c.runtime.Home, "jdk/bin/java"), "-Xms16m", "-Xmx64m", "-cp", c.runtime.Home,
		"Scout", "inject", strconv.Itoa(t.PID), t.Start, t.UID, t.GID, strings.Join(options, ";"))
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "Unknown", "Attachment interrupted; no automatic retry for this process"
	}
	if err != nil {
		return "Blocked", "Attachment failed; see collector log. No automatic retry for this process"
	}
	return "Attached", "Instrumentation acknowledged"
}
