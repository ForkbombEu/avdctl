package sshclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestSplitTarget(t *testing.T) {
	host, port, user, err := splitTarget("alice@example.com:2222")
	if err != nil {
		t.Fatalf("splitTarget error: %v", err)
	}
	if host != "example.com" || port != 2222 || user != "alice" {
		t.Fatalf("unexpected split: host=%q port=%d user=%q", host, port, user)
	}
}

func TestParseArgs(t *testing.T) {
	got, err := parseArgs([]string{
		"-p", "2200",
		"-l", "bob",
		"-i", "~/.ssh/id_ed25519",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=~/.ssh/kh",
	})
	if err != nil {
		t.Fatalf("parseArgs error: %v", err)
	}
	if got.Port != 2200 || got.User != "bob" {
		t.Fatalf("unexpected basics: %#v", got)
	}
	if len(got.IdentityFiles) != 1 {
		t.Fatalf("missing identity files: %#v", got.IdentityFiles)
	}
	if !got.InsecureIgnoreHost {
		t.Fatalf("StrictHostKeyChecking=no should set insecure mode")
	}
	if len(got.KnownHostsFiles) != 1 {
		t.Fatalf("missing known_hosts files: %#v", got.KnownHostsFiles)
	}
}

func TestKnownHostKeyAlgorithmsUsesTrustedKeyTypes(t *testing.T) {
	keys := []knownhosts.KnownKey{
		{Key: testPublicKey{keyType: ssh.KeyAlgoED25519}},
		{Key: testPublicKey{keyType: ssh.KeyAlgoRSA}},
		{Key: testPublicKey{keyType: ssh.KeyAlgoED25519}},
	}

	got := knownHostKeyAlgorithms(keys)
	want := []string{ssh.KeyAlgoED25519, ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	if len(got) != len(want) {
		t.Fatalf("algorithm count = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("algorithm[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDialRetriesWithTrustedHostKeyAlgorithm(t *testing.T) {
	_, edPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	edSigner, err := ssh.NewSignerFromKey(edPrivate)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaPrivate, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	ecdsaSigner, err := ssh.NewSignerFromKey(ecdsaPrivate)
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	serverConfig := &ssh.ServerConfig{
		PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	serverConfig.AddHostKey(ecdsaSigner)
	serverConfig.AddHostKey(edSigner)
	accepted := make(chan struct{}, 2)
	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}
			accepted <- struct{}{}
			go func() {
				serverConn, channels, requests, handshakeErr := ssh.NewServerConn(conn, serverConfig)
				if handshakeErr != nil {
					return
				}
				defer serverConn.Close()
				go ssh.DiscardRequests(requests)
				for channel := range channels {
					_ = channel.Reject(ssh.UnknownChannelType, "not used by this test")
				}
			}()
		}
	}()

	host, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	knownHostsPath := filepath.Join(t.TempDir(), "known_hosts")
	knownHost := fmt.Sprintf("[%s]:%s %s", host, port, ssh.MarshalAuthorizedKey(edSigner.PublicKey()))
	if err := os.WriteFile(knownHostsPath, []byte(knownHost), 0o600); err != nil {
		t.Fatal(err)
	}

	client, err := dial(context.Background(), "test@"+listener.Addr().String(), []string{
		"-o", "UserKnownHostsFile=" + knownHostsPath,
	}, "password")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	for attempt := 0; attempt < 2; attempt++ {
		select {
		case <-accepted:
		case <-time.After(time.Second):
			t.Fatalf("SSH attempts = %d, want 2", attempt)
		}
	}
}

type testPublicKey struct {
	keyType string
}

func (key testPublicKey) Type() string                    { return key.keyType }
func (testPublicKey) Marshal() []byte                     { return nil }
func (testPublicKey) Verify([]byte, *ssh.Signature) error { return nil }
