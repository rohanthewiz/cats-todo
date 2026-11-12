package main

import (
	"os"
	"strings"
	"testing"
)

// pasteboardEnv must win over whatever locale the pane inherited, including one
// that is set but not UTF-8: pbpaste falls back to MacRoman when ANY of LANG,
// LC_ALL or LC_CTYPE is non-UTF-8, so overriding LANG alone is not enough.
func TestPasteboardEnvPinsUTF8(t *testing.T) {
	t.Setenv("LANG", "C")
	t.Setenv("LC_ALL", "C")
	t.Setenv("LC_CTYPE", "C")

	// os/exec keeps the last value of a duplicated key; resolve the same way.
	last := map[string]string{}
	for _, kv := range pasteboardEnv() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			last[k] = v
		}
	}
	for _, k := range []string{"LANG", "LC_ALL", "LC_CTYPE"} {
		if !strings.HasSuffix(last[k], ".UTF-8") {
			t.Errorf("%s = %q under pasteboardEnv, want a UTF-8 locale", k, last[k])
		}
	}
	if last["HOME"] != os.Getenv("HOME") {
		t.Error("pasteboardEnv dropped the inherited environment")
	}
}
