//go:build internaltools && cgo

// Package main implements a lightweight SQL execution helper that reuses the
// as400 protocol client. The binary is only built when the `internaltools` build
// tag is provided and is therefore excluded from production distributions.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	as400module "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/as400"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/pkg/dbdriver"
	as400proto "github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/as400"
	"gopkg.in/yaml.v3"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to ibm.d configuration file")
		jobName    = flag.String("job", "", "Job name defined in configuration (required)")
		timeout    = flag.Duration("timeout", 30*time.Second, "Overall timeout for SQL execution")
		jsonOut    = flag.Bool("json", false, "Emit query results as JSON")
	)
	flag.Parse()

	if *jobName == "" {
		fatal("job name is required (use -job)")
	}
	if flag.NArg() == 0 {
		fatal("SQL statement is required as positional argument")
	}
	statement := strings.Join(flag.Args(), " ")

	jobs, err := loadJobs(*configPath)
	if err != nil {
		fatal("load config: %v", err)
	}
	jobCfg, ok := jobs[*jobName]
	if !ok {
		fatal("job %q not found", *jobName)
	}

	dsn, err := jobCfg.buildDSN()
	if err != nil {
		fatal("build DSN: %v", err)
	}

	client := as400proto.NewClient(as400proto.Config{
		DSN:          dsn,
		Timeout:      jobCfg.timeoutOrDefault(),
		MaxOpenConns: jobCfg.maxOpenConnsOrDefault(),
		ConnMaxLife:  jobCfg.maxConnLifeOrDefault(),
	})

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if err := client.Connect(ctx); err != nil {
		fatal("connect: %v", err)
	}
	defer func() { _ = client.Close() }()

	rows := make([]map[string]string, 0)
	err = client.Query(ctx, statement, func(columns []string, values []string) error {
		row := make(map[string]string, len(columns))
		for i, col := range columns {
			row[col] = values[i]
		}
		rows = append(rows, row)
		return nil
	})
	if err != nil {
		fatal("query failed: %v", err)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fatal("encode JSON: %v", err)
		}
		return
	}

	for _, row := range rows {
		for col, val := range row {
			fmt.Printf("%s=%s\n", col, val)
		}
		fmt.Println("--")
	}
}

type jobConfig struct {
	Name               string `yaml:"name"`
	Module             string `yaml:"module"`
	as400module.Config `yaml:",inline"`
}

func loadJobs(path string) (map[string]jobConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw struct {
		Jobs []jobConfig `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	jobs := make(map[string]jobConfig)
	for _, job := range raw.Jobs {
		if job.Name == "" {
			continue
		}
		if job.Module != "" && job.Module != "as400" {
			continue
		}
		jobs[job.Name] = job
	}
	if len(jobs) == 0 {
		return nil, fmt.Errorf("no jobs found in %s", path)
	}
	return jobs, nil
}

func (j jobConfig) timeoutOrDefault() time.Duration {
	if d := time.Duration(j.Timeout); d > 0 {
		return d
	}
	return 2 * time.Second
}

func (j jobConfig) maxOpenConnsOrDefault() int {
	if j.MaxDbConns > 0 {
		return j.MaxDbConns
	}
	return 1
}

func (j jobConfig) maxConnLifeOrDefault() time.Duration {
	if d := time.Duration(j.MaxDbLifeTime); d > 0 {
		return d
	}
	return 10 * time.Minute
}

func (j jobConfig) buildDSN() (string, error) {
	if strings.TrimSpace(j.DSN) != "" {
		return j.DSN, nil
	}
	if j.Hostname == "" || j.Username == "" || j.Password == "" {
		return "", fmt.Errorf("job %q missing DSN or connection parameters", j.Name)
	}
	port := j.Port
	if port == 0 {
		if j.UseSSL {
			port = 446
		} else {
			port = 8471
		}
	}
	conn := &dbdriver.ConnectionConfig{
		Hostname:   j.Hostname,
		Port:       port,
		Username:   j.Username,
		Password:   j.Password,
		Database:   fallback(j.Database, "*SYSBAS"),
		SystemType: "AS400",
		ODBCDriver: fallback(j.ODBCDriver, "IBM i Access ODBC Driver"),
		UseSSL:     j.UseSSL,
	}
	return dbdriver.BuildODBCDSN(conn), nil
}

func fallback(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "ibmi-sql: "+format+"\n", args...)
	os.Exit(1)
}
