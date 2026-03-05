// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
)

func commandWithEnv(extraEnv []string, bin string, args ...string) *exec.Cmd {
	cmd := exec.Command(bin, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd
}

func commandContextWithEnv(ctx context.Context, extraEnv []string, bin string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, bin, args...)
	if len(extraEnv) > 0 {
		cmd.Env = append(os.Environ(), extraEnv...)
	}
	return cmd
}

func runCommandWithEnv(
	ctx context.Context,
	extraEnv []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	bin string,
	args ...string,
) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := commandContextWithEnv(ctx, extraEnv, bin, args...)
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func runCommandOutputWithEnv(
	ctx context.Context,
	extraEnv []string,
	stdin io.Reader,
	bin string,
	args ...string,
) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := commandContextWithEnv(ctx, extraEnv, bin, args...)
	cmd.Stdin = stdin
	var out bytes.Buffer
	var errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	err := cmd.Run()
	return out.String(), errOut.String(), err
}

func runCommandCombinedOutputWithEnv(
	ctx context.Context,
	extraEnv []string,
	stdin io.Reader,
	bin string,
	args ...string,
) ([]byte, error) {
	out, errOut, err := runCommandOutputWithEnv(ctx, extraEnv, stdin, bin, args...)
	combined := out + errOut
	if err != nil {
		return []byte(combined), err
	}
	return []byte(combined), nil
}
