// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package remoteavdctl

import (
	"bytes"
	"context"
	"io"

	"github.com/forkbombeu/avdctl/internal/sshclient"
)

func remoteArgv(avdArgs []string) []string {
	argv := make([]string, 0, len(avdArgs)+4)
	// Avoid recursive delegation on the remote side if these env vars are set there.
	argv = append(argv, "env", "AVDCTL_SSH_TARGET=", "AVDCTL_SSH_ARGS=", "avdctl")
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
	return sshclient.RunArgs(ctx, target, sshArgs, remoteArgv(avdArgs), stdin, stdout, stderr, tty)
}

// RunOutput delegates an avdctl command and captures stdout/stderr.
func RunOutput(ctx context.Context, target string, sshArgs []string, avdArgs []string) (string, string, error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Run(ctx, target, sshArgs, avdArgs, nil, &out, &errOut, false)
	return out.String(), errOut.String(), err
}
