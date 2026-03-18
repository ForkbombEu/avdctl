package main

import (
	"errors"
	"fmt"
	"strings"

	redroidcore "github.com/forkbombeu/avdctl/internal/redroid"
)

func startRedroidWithOutput(env redroidcore.Env, opts redroidcore.StartOptions) error {
	if strings.TrimSpace(opts.Name) == "" {
		return errors.New("--name is required")
	}
	containerID, err := redroidcore.Start(env, opts)
	if err != nil {
		return err
	}
	if containerID == "" {
		fmt.Printf("Started %s\n", opts.Name)
		return nil
	}
	fmt.Printf("Started %s (%s)\n", opts.Name, containerID)
	return nil
}

func waitForRedroidWithOutput(env redroidcore.Env, opts redroidcore.WaitOptions) error {
	if err := redroidcore.WaitForBoot(env, opts); err != nil {
		return err
	}
	fmt.Printf("Redroid ready on %s\n", opts.Serial)
	return nil
}

func stopRedroidWithOutput(env redroidcore.Env, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("--name is required")
	}
	if err := redroidcore.Stop(env, name); err != nil {
		return err
	}
	fmt.Printf("Stopped %s\n", name)
	return nil
}

func deleteRedroidWithOutput(env redroidcore.Env, name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("--name is required")
	}
	if err := redroidcore.Delete(env, name); err != nil {
		return err
	}
	fmt.Printf("Deleted %s\n", name)
	return nil
}
