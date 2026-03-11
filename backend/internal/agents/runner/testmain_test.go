package runner

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	tmpHome, err := os.MkdirTemp("", "runner-test-home-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpHome)

	origHome := os.Getenv("HOME")
	if err := os.Setenv("HOME", tmpHome); err != nil {
		panic(err)
	}
	defer os.Setenv("HOME", origHome)

	os.Exit(m.Run())
}
