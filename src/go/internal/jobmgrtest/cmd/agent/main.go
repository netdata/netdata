package main

import (
	"os"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/agentdriver"
)

func main() {
	os.Exit(agentdriver.Main(os.Args[1:]))
}
