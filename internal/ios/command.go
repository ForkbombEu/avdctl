// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package ios

import (
	"bytes"
	"context"
	"io"
	"os/exec"
)

func commandContext(ctx context.Context, bin string, args ...string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	return exec.CommandContext(ctx, bin, args...)
}

func runCommandOutput(
	ctx context.Context,
	stdin io.Reader,
	bin string,
	args ...string,
) (string, string, error) {
	cmd := commandContext(ctx, bin, args...)
	cmd.Stdin = stdin
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}
