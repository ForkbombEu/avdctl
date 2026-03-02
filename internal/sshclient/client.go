// Copyright (C) 2025 Forkbomb B.V.
// License: AGPL-3.0-only

package sshclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

type parsedArgs struct {
	User               string
	Port               int
	IdentityFiles      []string
	KnownHostsFiles    []string
	InsecureIgnoreHost bool
}

// Run executes a remote command over SSH and streams stdio.
func Run(
	ctx context.Context,
	target string,
	sshArgs []string,
	command string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) error {
	client, err := dial(ctx, target, sshArgs)
	if err != nil {
		return err
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	session.Stdin = stdin
	session.Stdout = stdout
	session.Stderr = stderr

	if tty {
		modes := ssh.TerminalModes{
			ssh.ECHO:          1,
			ssh.TTY_OP_ISPEED: 14400,
			ssh.TTY_OP_OSPEED: 14400,
		}
		_ = session.RequestPty("xterm-256color", 24, 80, modes)
	}

	return runSession(ctx, client, session, command)
}

// RunArgs executes argv via `sh -lc` on the remote host and streams stdio.
func RunArgs(
	ctx context.Context,
	target string,
	sshArgs []string,
	argv []string,
	stdin io.Reader,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) error {
	return Run(ctx, target, sshArgs, shellWrap(shellJoin(argv)), stdin, stdout, stderr, tty)
}

// RunOutput executes a remote command string over SSH and returns stdout/stderr.
func RunOutput(ctx context.Context, target string, sshArgs []string, command string) (string, string, error) {
	var out bytes.Buffer
	var errOut bytes.Buffer
	err := Run(ctx, target, sshArgs, command, nil, &out, &errOut, false)
	return out.String(), errOut.String(), err
}

// RunOutputArgs executes argv via `sh -lc` on the remote host and returns stdout/stderr.
func RunOutputArgs(ctx context.Context, target string, sshArgs []string, argv []string) (string, string, error) {
	return RunOutput(ctx, target, sshArgs, shellWrap(shellJoin(argv)))
}

// RunDetachedArgs starts argv in the remote background, redirects output to logPath, and returns its PID.
func RunDetachedArgs(
	ctx context.Context,
	target string,
	sshArgs []string,
	argv []string,
	logPath string,
) (int, string, error) {
	script := "nohup " + shellJoin(argv) + " >> " + shellQuote(logPath) + " 2>&1 < /dev/null & echo $!"
	out, errOut, err := RunOutput(ctx, target, sshArgs, "sh -lc "+shellQuote(script))
	if err != nil {
		return 0, errOut, err
	}
	pidStr := strings.TrimSpace(out)
	pid, convErr := strconv.Atoi(pidStr)
	if convErr != nil || pid <= 0 {
		if errOut != "" {
			return 0, errOut, fmt.Errorf("failed to parse remote pid from output %q", pidStr)
		}
		return 0, "", fmt.Errorf("failed to parse remote pid from output %q", pidStr)
	}
	return pid, errOut, nil
}

func shellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, shellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func shellWrap(command string) string {
	return "sh -lc " + shellQuote(command)
}

func dial(ctx context.Context, target string, sshArgs []string) (*ssh.Client, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil, errors.New("empty ssh target")
	}

	host, port, defaultUser, err := splitTarget(target)
	if err != nil {
		return nil, err
	}
	parsed, err := parseArgs(sshArgs)
	if err != nil {
		return nil, err
	}
	userName := defaultUser
	if strings.TrimSpace(parsed.User) != "" {
		userName = strings.TrimSpace(parsed.User)
	}
	if userName == "" {
		u, err := user.Current()
		if err != nil {
			return nil, fmt.Errorf("resolve local user: %w", err)
		}
		userName = u.Username
	}
	if parsed.Port > 0 {
		port = parsed.Port
	}

	auth, err := authMethods(parsed.IdentityFiles)
	if err != nil {
		return nil, err
	}
	if len(auth) == 0 {
		return nil, errors.New("no ssh auth method available (agent, key files, or AVDCTL_SSH_PASSWORD)")
	}

	hostKeyCallback, err := hostKeyVerifier(parsed)
	if err != nil {
		return nil, err
	}

	config := &ssh.ClientConfig{
		User:            userName,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	address := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("ssh dial %s: %w", address, err)
	}

	cConn, chans, reqs, err := ssh.NewClientConn(conn, address, config)
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("ssh handshake %s: %w", address, err)
	}
	return ssh.NewClient(cConn, chans, reqs), nil
}

func runSession(ctx context.Context, client *ssh.Client, session *ssh.Session, command string) error {
	done := make(chan error, 1)
	go func() {
		done <- session.Run(command)
	}()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		_ = client.Close()
		<-done
		return ctx.Err()
	}
}

func splitTarget(target string) (host string, port int, userName string, err error) {
	port = 22
	at := strings.LastIndex(target, "@")
	if at >= 0 {
		userName = target[:at]
		target = target[at+1:]
	}

	// URL-like parsing first for robustness with IPv6 and explicit ports.
	if strings.Contains(target, ":") {
		u, parseErr := url.Parse("ssh://" + target)
		if parseErr == nil && u.Hostname() != "" {
			host = u.Hostname()
			if p := u.Port(); p != "" {
				n, convErr := strconv.Atoi(p)
				if convErr != nil {
					return "", 0, "", fmt.Errorf("invalid ssh port %q", p)
				}
				port = n
			}
			return host, port, userName, nil
		}
	}

	host = target
	if host == "" {
		return "", 0, "", errors.New("empty ssh host")
	}
	return host, port, userName, nil
}

func parseArgs(args []string) (parsedArgs, error) {
	out := parsedArgs{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-p":
			if i+1 >= len(args) {
				return out, errors.New("missing value for -p")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil {
				return out, fmt.Errorf("invalid ssh port %q", args[i])
			}
			out.Port = n
		case strings.HasPrefix(arg, "-p") && len(arg) > 2:
			n, err := strconv.Atoi(arg[2:])
			if err != nil {
				return out, fmt.Errorf("invalid ssh port %q", arg[2:])
			}
			out.Port = n
		case arg == "-l":
			if i+1 >= len(args) {
				return out, errors.New("missing value for -l")
			}
			i++
			out.User = args[i]
		case arg == "-i":
			if i+1 >= len(args) {
				return out, errors.New("missing value for -i")
			}
			i++
			out.IdentityFiles = append(out.IdentityFiles, expandHome(args[i]))
		case strings.HasPrefix(arg, "-i") && len(arg) > 2:
			out.IdentityFiles = append(out.IdentityFiles, expandHome(arg[2:]))
		case arg == "-o":
			if i+1 >= len(args) {
				return out, errors.New("missing value for -o")
			}
			i++
			applyOpenSSHOption(&out, args[i])
		case strings.HasPrefix(arg, "-o") && len(arg) > 2:
			applyOpenSSHOption(&out, arg[2:])
		case arg == "-t" || arg == "-tt" || arg == "-T":
			// handled by caller; accepted for compatibility.
			continue
		default:
			// Ignore unsupported args for forward compatibility with existing callers.
			continue
		}
	}
	return out, nil
}

func applyOpenSSHOption(out *parsedArgs, opt string) {
	key, val, ok := strings.Cut(opt, "=")
	if !ok {
		return
	}
	key = strings.ToLower(strings.TrimSpace(key))
	val = strings.TrimSpace(val)
	switch key {
	case "stricthostkeychecking":
		if strings.EqualFold(val, "no") || strings.EqualFold(val, "off") || strings.EqualFold(val, "false") {
			out.InsecureIgnoreHost = true
		}
	case "userknownhostsfile":
		if val != "" {
			for _, p := range strings.Split(val, ",") {
				p = strings.TrimSpace(p)
				if p != "" {
					out.KnownHostsFiles = append(out.KnownHostsFiles, expandHome(p))
				}
			}
		}
	case "identityfile":
		if val != "" {
			out.IdentityFiles = append(out.IdentityFiles, expandHome(val))
		}
	}
}

func authMethods(identityFiles []string) ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod

	if pass := os.Getenv("AVDCTL_SSH_PASSWORD"); pass != "" {
		methods = append(methods, ssh.Password(pass))
	}

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			agentClient := agent.NewClient(conn)
			methods = append(methods, ssh.PublicKeysCallback(agentClient.Signers))
		}
	}

	if len(identityFiles) == 0 {
		home, _ := os.UserHomeDir()
		if home != "" {
			identityFiles = []string{
				filepath.Join(home, ".ssh", "id_ed25519"),
				filepath.Join(home, ".ssh", "id_ecdsa"),
				filepath.Join(home, ".ssh", "id_rsa"),
			}
		}
	}
	for _, path := range identityFiles {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		signer, err := ssh.ParsePrivateKey(b)
		if err != nil {
			continue
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	return methods, nil
}

func hostKeyVerifier(parsed parsedArgs) (ssh.HostKeyCallback, error) {
	if parsed.InsecureIgnoreHost {
		return ssh.InsecureIgnoreHostKey(), nil
	}

	knownHosts := parsed.KnownHostsFiles
	if len(knownHosts) == 0 {
		home, _ := os.UserHomeDir()
		if home != "" {
			knownHosts = []string{filepath.Join(home, ".ssh", "known_hosts")}
		}
	}
	if len(knownHosts) == 0 {
		return nil, errors.New("no known_hosts file found")
	}
	return knownhosts.New(knownHosts...)
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
