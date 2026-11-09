package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunExitCodes(t *testing.T) {
	cases := []struct {
		args []string
		code int
		out  string
	}{
		{nil, 2, ""},
		{[]string{"--help"}, 0, ""},
		{[]string{"nope"}, 2, ""},
		{[]string{"version"}, 0, "homebase dev\n"},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		if got := run(c.args, &out, &errb); got != c.code {
			t.Errorf("run(%v) = %d, want %d", c.args, got, c.code)
		}
		if c.out != "" && out.String() != c.out {
			t.Errorf("run(%v) stdout %q, want %q", c.args, out.String(), c.out)
		}
		if c.code == 2 && !strings.Contains(errb.String(), "usage:") {
			t.Errorf("run(%v) stderr lacks usage", c.args)
		}
	}
}
