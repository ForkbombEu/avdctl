// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package ios

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
)

func commandContext(ctx context.Context, bin string, args ...string) *exec.Cmd {
	if ctx == nil {
		ctx = context.Background()
	}
	return exec.CommandContext(ctx, bin, args...)
}

func runCommandOutput(
	env Env,
	stdin io.Reader,
	bin string,
	args ...string,
) (string, string, error) {
	ctx := spanContext(env)
	cmd := commandContext(ctx, bin, args...)
	cmd.Stdin = stdin
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = io.MultiWriter(&errOut, newCommandLogWriter(env, bin, args))
	logEvent(env, "command started", "command", bin, "args", strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		logEvent(
			env,
			"command failed",
			"command",
			bin,
			"args",
			strings.Join(args, " "),
			"error",
			err,
			"stderr",
			strings.TrimSpace(errOut.String()),
		)
	}
	return out.String(), errOut.String(), err
}
