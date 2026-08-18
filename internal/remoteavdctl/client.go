// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package remoteavdctl

import (
	"bytes"
	"context"
	"io"

	"github.com/forkbombeu/avdctl/internal/sshclient"
)

func remoteArgv(environment, avdArgs []string) []string {
	argv := make([]string, 0, len(environment)+len(avdArgs)+4)
	// Avoid recursive delegation on the remote side if these env vars are set there.
	argv = append(argv, "env", "AVDCTL_SSH_TARGET=", "AVDCTL_SSH_ARGS=")
	for _, entry := range environment {
		if entry != "" {
			argv = append(argv, entry)
		}
	}
	argv = append(argv, "avdctl")
	argv = append(argv, avdArgs...)
	return argv
}

// Run delegates an avdctl command to a remote host over SSH and streams stdio.
func Run(
	ctx context.Context,
	target string,
	sshArgs []string,
	avdArgs []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) error {
	return RunWithEnvironment(ctx, target, sshArgs, "", nil, avdArgs, stdin, stdout, stderr, tty)
}

// RunWithEnvironment delegates an avdctl command with connection-scoped SSH
// authentication and remote-only environment values.
func RunWithEnvironment(
	ctx context.Context,
	target string,
	sshArgs []string,
	sshPassword string,
	environment []string,
	avdArgs []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) error {
	return sshclient.RunArgsWithPassword(ctx, target, sshArgs, sshPassword, remoteArgv(environment, avdArgs), stdin, stdout, stderr, tty)
}

// RunOutput delegates an avdctl command and captures stdout/stderr.
func RunOutput(ctx context.Context, target string, sshArgs []string, avdArgs []string) (string, string, error) {
	return RunOutputWithEnvironment(ctx, target, sshArgs, "", nil, avdArgs)
}

// RunOutputWithEnvironment delegates an avdctl command and captures output.
func RunOutputWithEnvironment(ctx context.Context, target string, sshArgs []string, sshPassword string, environment, avdArgs []string) (string, string, error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := RunWithEnvironment(ctx, target, sshArgs, sshPassword, environment, avdArgs, nil, &out, &errOut, false)
	return out.String(), errOut.String(), err
}
