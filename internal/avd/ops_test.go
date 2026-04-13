package avd

import "testing"

func TestDetect(t *testing.T) {
	env := Detect()
	if env.AVDHome == "" {
		t.Fatal("AVDHome should not be empty")
	}
}
