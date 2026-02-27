// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package avd

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

func usesSSH(env Env) bool {
	return strings.TrimSpace(env.SSHTarget) != ""
}

func command(env Env, bin string, args ...string) *exec.Cmd {
	return commandWithEnv(env, nil, bin, args...)
}

func commandContext(ctx context.Context, env Env, bin string, args ...string) *exec.Cmd {
	return commandContextWithEnv(ctx, env, nil, bin, args...)
}

func commandWithEnv(env Env, extraEnv []string, bin string, args ...string) *exec.Cmd {
	if !usesSSH(env) {
		cmd := exec.Command(bin, args...)
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
		return cmd
	}
	sshArgs := buildSSHArgs(env, extraEnv, bin, args...)
	return exec.Command(sshBin(env), sshArgs...)
}

func commandContextWithEnv(ctx context.Context, env Env, extraEnv []string, bin string, args ...string) *exec.Cmd {
	if !usesSSH(env) {
		cmd := exec.CommandContext(ctx, bin, args...)
		if len(extraEnv) > 0 {
			cmd.Env = append(os.Environ(), extraEnv...)
		}
		return cmd
	}
	sshArgs := buildSSHArgs(env, extraEnv, bin, args...)
	return exec.CommandContext(ctx, sshBin(env), sshArgs...)
}

func buildSSHArgs(env Env, extraEnv []string, bin string, args ...string) []string {
	cmdArgs := make([]string, 0, len(extraEnv)+1+len(args))
	if len(extraEnv) > 0 {
		cmdArgs = append(cmdArgs, "env")
		cmdArgs = append(cmdArgs, extraEnv...)
	}
	cmdArgs = append(cmdArgs, bin)
	cmdArgs = append(cmdArgs, args...)

	remoteCommand := shellJoin(cmdArgs)
	sshArgs := make([]string, 0, len(env.SSHArgs)+4)
	sshArgs = append(sshArgs, env.SSHArgs...)
	sshArgs = append(sshArgs, env.SSHTarget, "sh", "-lc", remoteCommand)
	return sshArgs
}

func sshBin(env Env) string {
	if strings.TrimSpace(env.SSHBin) == "" {
		return "ssh"
	}
	return env.SSHBin
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
