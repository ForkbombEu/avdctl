// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/forkbombeu/avdctl/internal/sshclient"
)

func usesSSH(env Env) bool {
	return strings.TrimSpace(env.SSHTarget) != ""
}

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

func buildRemoteArgv(extraEnv []string, bin string, args ...string) []string {
	cmdArgs := make([]string, 0, len(extraEnv)+1+len(args))
	if len(extraEnv) > 0 {
		cmdArgs = append(cmdArgs, "env")
		cmdArgs = append(cmdArgs, extraEnv...)
	}
	cmdArgs = append(cmdArgs, bin)
	cmdArgs = append(cmdArgs, args...)
	return cmdArgs
}

func runCommandWithEnv(
	ctx context.Context,
	env Env,
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
	if !usesSSH(env) {
		cmd := commandContextWithEnv(ctx, extraEnv, bin, args...)
		cmd.Stdin = stdin
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		return cmd.Run()
	}
	argv := buildRemoteArgv(extraEnv, bin, args...)
	return sshclient.RunArgs(ctx, env.SSHTarget, env.SSHArgs, argv, stdin, stdout, stderr, false)
}

func runCommandOutputWithEnv(
	ctx context.Context,
	env Env,
	extraEnv []string,
	stdin io.Reader,
	bin string,
	args ...string,
) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if !usesSSH(env) {
		cmd := commandContextWithEnv(ctx, extraEnv, bin, args...)
		cmd.Stdin = stdin
		var out bytes.Buffer
		var errOut bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errOut
		err := cmd.Run()
		return out.String(), errOut.String(), err
	}
	argv := buildRemoteArgv(extraEnv, bin, args...)
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := sshclient.RunArgs(ctx, env.SSHTarget, env.SSHArgs, argv, stdin, &out, &errOut, false)
	return out.String(), errOut.String(), err
}

func runCommandCombinedOutputWithEnv(
	ctx context.Context,
	env Env,
	extraEnv []string,
	stdin io.Reader,
	bin string,
	args ...string,
) ([]byte, error) {
	out, errOut, err := runCommandOutputWithEnv(ctx, env, extraEnv, stdin, bin, args...)
	combined := out + errOut
	if err != nil {
		return []byte(combined), err
	}
	return []byte(combined), nil
}
