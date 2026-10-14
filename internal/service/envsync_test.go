// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import "testing"

func TestEnvStatus(t *testing.T) {
	cases := []struct {
		name, app, server string
		exists            bool
		want              string
	}{
		{"same", "A=1\nB=2\n", "A=1\nB=2\n", true, "in-sync"},
		{"trailing newline", "A=1\nB=2", "A=1\nB=2\n\n", true, "in-sync"},
		{"CRLF", "A=1\nB=2\n", "A=1\r\nB=2\r\n", true, "in-sync"},
		{"changed value", "A=1\nB=2\n", "A=1\nB=3\n", true, "differs"},
		{"added line", "A=1\n", "A=1\nC=9\n", true, "differs"},
		{"neither has one", "", "", false, "in-sync"},
		{"app empty, server empty file", "  \n", "", true, "in-sync"},
		{"not written yet", "A=1\n", "", false, "not-on-server"},
		{"only on the server", "", "A=1\n", true, "only-on-server"},
	}
	for _, c := range cases {
		if got := envStatus(c.app, c.server, c.exists); got != c.want {
			t.Errorf("%s: envStatus = %s, want %s", c.name, got, c.want)
		}
	}
}
