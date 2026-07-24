// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "context"

func frameTaskWork(work func(context.Context) (SealedResult, error)) TaskWork {
	return func(ctx context.Context) (TaskOutcome, error) {
		result, err := work(ctx)
		if err != nil {
			return TaskOutcome{}, err
		}
		return NewFrameOutcome(result)
	}
}
