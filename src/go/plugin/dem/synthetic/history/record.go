// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/systemd-journal-sdk/go/journal"
)

// AppendRun writes one immutable start/completion record. An attempted
// append error has an uncertain disk outcome and must never trigger replay.
func (s *Store) AppendRun(ctx context.Context, phase string, run synthetic.Run) (bool, error) {
	if phase != "start" && phase != "complete" {
		return false, fmt.Errorf("invalid synthetic history phase")
	}
	if run.ID == "" || run.JobID == "" || run.StartedUS <= 0 {
		return false, fmt.Errorf("invalid synthetic history identity")
	}
	if phase == "complete" && run.CompletedUS < run.StartedUS {
		return false, fmt.Errorf("invalid synthetic completion time")
	}
	data, err := json.Marshal(run)
	if err != nil {
		return false, err
	}
	return s.journal.Append(ctx, []journal.Field{
		journal.StringField("DEM_KIND", "synthetic"), journal.StringField("DEM_PHASE", phase),
		journal.StringField("DEM_JOB_ID", run.JobID), journal.StringField("DEM_RUN_ID", run.ID),
		journal.StringField("DEM_DATA", string(data)), journal.StringField("MESSAGE", "synthetic "+phase),
	})
}
