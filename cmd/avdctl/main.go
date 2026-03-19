// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/forkbombeu/avdctl/pkg/avdmanager"
)

var version = "dev"
var errRemoteDelegated = errors.New("command delegated to remote avdctl")

const colophon = `
                 _      _   _ _
  __ ___   ____| | ___| |_| | |
 / _` + "`" + ` \ \ / / _` + "`" + ` |/ __| __| | |
| (_| |\ V / (_| | (__| |_| | |
 \__,_| \_/ \__,_|\___|\__|_|_|
`

func main() {
	shutdownTracing, err := avdmanager.SetupTracing(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize tracing: %v\n", err)
	}
	if shutdownTracing != nil {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := shutdownTracing(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "failed to shutdown tracing: %v\n", err)
			}
		}()
	}
	root := newRootCommand(version)
	if err := root.Execute(); err != nil {
		if isRemoteDelegatedError(err) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func isRemoteDelegatedError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errRemoteDelegated) {
		return true
	}
	return strings.Contains(err.Error(), errRemoteDelegated.Error())
}
