// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package gitlabctl

import (
	"reflect"
	"testing"
)

func TestIsGitLabImage(t *testing.T) {
	for image, want := range map[string]bool{
		"gitlab/gitlab-ce:17.5.1-ce.0":             true,
		"registry.example.com/gitlab/gitlab-ee:17": true,
		"gitlab/gitlab-jh:latest":                  true,
		"gitlab/gitlab-runner:latest":              false,
		"nginx:alpine":                             false,
	} {
		if got := IsGitLabImage(image); got != want {
			t.Errorf("IsGitLabImage(%q) = %v, want %v", image, got, want)
		}
	}
}

// The line shapes are runit's, verified against a real "sv status": a
// running service, a stopped one ("normally up"), and gitlab-ctl's form
// with the service directory already stripped to its name.
func TestParseStatus(t *testing.T) {
	out := `run: gitaly: (pid 311) 4200s; run: log: (pid 300) 4201s
down: puma: 12s, normally up; run: log: (pid 301) 4201s
run: /opt/gitlab/service/sidekiq: (pid 322) 4190s
fail: redis: runsv not running
warning: something unrelated
`
	want := []ServiceState{{"gitaly", true}, {"puma", false}, {"sidekiq", true}, {"redis", false}}
	if got := ParseStatus(out); !reflect.DeepEqual(got, want) {
		t.Errorf("ParseStatus = %+v, want %+v", got, want)
	}
	if got := ParseStatus("OCI runtime exec failed: executable file not found"); len(got) != 0 {
		t.Errorf("non-status output parsed as %+v", got)
	}
}
