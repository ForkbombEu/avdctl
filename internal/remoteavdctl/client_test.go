package remoteavdctl

import "testing"

func TestRemoteArgvKeepsDelegationScoped(t *testing.T) {
	argv := remoteArgv([]string{"AVDCTL_SUDO_PASSWORD=secret"}, []string{"run", "redroid"})
	want := []string{"env", "AVDCTL_SSH_TARGET=", "AVDCTL_SSH_ARGS=", "AVDCTL_SUDO_PASSWORD=secret", "avdctl", "run", "redroid"}
	if len(argv) != len(want) {
		t.Fatalf("argv = %#v", argv)
	}
	for i := range want {
		if argv[i] != want[i] {
			t.Fatalf("argv[%d] = %q, want %q", i, argv[i], want[i])
		}
	}
}
