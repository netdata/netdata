// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"os"
)

func readSecretFile(_ context.Context, path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return readBoundedSecret(file, MaximumAtomicResolvedBytes)
}
