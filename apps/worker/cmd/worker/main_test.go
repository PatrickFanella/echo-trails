package main

import (
	"os"
	"testing"
)

func TestEnvBoolDefaultsAndExplicitDisable(t *testing.T) {
	const name = "ECHOTRAILS_TEST_BOOL"
	t.Cleanup(func() { _ = os.Unsetenv(name) })
	_ = os.Unsetenv(name)
	if !envBool(name, true) {
		t.Fatal("missing value should preserve the enabled production default")
	}
	for _, value := range []string{"0", "false", "FALSE", "no", "NO"} {
		if err := os.Setenv(name, value); err != nil {
			t.Fatal(err)
		}
		if envBool(name, true) {
			t.Fatalf("%q should disable the feature", value)
		}
	}
}
