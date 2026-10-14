// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFiles(t *testing.T) {
	ok := []File{{Path: EntryPoint, Content: "- hosts: all\n"}, {Path: "roles/web/tasks/main.yml", Content: "- debug: msg=hi\n"}}
	if err := ValidateFiles(ok, EntryPoint); err != nil {
		t.Fatal(err)
	}
	for name, bad := range map[string][]File{
		"no entry point": {{Path: "other.yml", Content: "x"}},
		"empty entry":    {{Path: EntryPoint, Content: "  \n"}},
		"escapes":        {{Path: EntryPoint, Content: "x"}, {Path: "../etc/passwd", Content: "x"}},
		"absolute":       {{Path: EntryPoint, Content: "x"}, {Path: "/etc/passwd", Content: "x"}},
		"reserved":       {{Path: EntryPoint, Content: "x"}, {Path: "inventory.ini", Content: "x"}},
		"twice":          {{Path: EntryPoint, Content: "x"}, {Path: EntryPoint, Content: "y"}},
		"binary":         {{Path: EntryPoint, Content: "x"}, {Path: "a.bin", Content: "a\x00b"}},
	} {
		if ValidateFiles(bad, EntryPoint) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestContentSHA(t *testing.T) {
	a := []File{{Path: EntryPoint, Content: "x"}, {Path: "b.yml", Content: "y"}}
	b := []File{{Path: "b.yml", Content: "y"}, {Path: EntryPoint, Content: "x"}}
	if ContentSHA(a, EntryPoint, "v: 1") != ContentSHA(b, EntryPoint, "v: 1\n") {
		t.Error("the same files in another order, or vars with trailing space, should hash the same")
	}
	if ContentSHA(a, EntryPoint, "v: 1") == ContentSHA(a, EntryPoint, "v: 2") || ContentSHA(a, EntryPoint, "") == ContentSHA([]File{{Path: EntryPoint, Content: "x!"}, {Path: "b.yml", Content: "y"}}, EntryPoint, "") || ContentSHA(a, EntryPoint, "") == ContentSHA(a, "b.yml", "") {
		t.Error("different vars or content should hash differently")
	}
}

func TestInventory(t *testing.T) {
	inv, targets, env, err := inventory([]Login{
		{Target: Target{ServerID: "a", Name: "web 1", Host: "10.0.0.1", User: "root"}, Port: 22, Password: "s3cret 'x'"},
		{Target: Target{ServerID: "b", Name: "web 1", Host: "10.0.0.2", User: "deploy"}, Port: 2222, Password: "p2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(inv, "s3cret") || strings.Contains(inv, "p2") {
		t.Errorf("a password is in the inventory file:\n%s", inv)
	}
	if !strings.Contains(inv, "web_1 ansible_host=10.0.0.1 ansible_port=22 ansible_user=root ansible_password=\"{{ lookup('env', 'PANEL_PW_0') }}\"") ||
		!strings.Contains(inv, "web_1_2 ansible_host=10.0.0.2 ansible_port=2222 ansible_user=deploy") {
		t.Errorf("inventory:\n%s", inv)
	}
	if targets[0].Name != "web_1" || targets[1].Name != "web_1_2" || env[0] != "PANEL_PW_0=s3cret 'x'" || env[1] != "PANEL_PW_1=p2" {
		t.Errorf("targets %+v env %v", targets, env)
	}
}

// A host or user that would add inventory variables of its own is refused;
// ordinary ones are kept.
func TestInventoryRefusesInjection(t *testing.T) {
	cases := []struct {
		host, user string
		ok         bool
	}{
		{"10.0.0.1", "root", true},
		{"web-1.example.com", "deploy", true},
		{"fe80::1%eth0", "svc.user@corp", true},
		{"2001:db8::7", "machine$", true},
		{"10.0.0.1 ansible_ssh_common_args='-o ProxyCommand=id'", "root", false},
		{"10.0.0.1", "root ansible_become_exe=sh", false},
		{"10.0.0.1", `CORP\alice`, false},
		{"10.0.0.1", `root"`, false},
		{"-oProxyCommand=id", "root", false},
		{"", "root", false},
		{"10.0.0.1", "", false},
	}
	for _, c := range cases {
		_, _, _, err := inventory([]Login{{Target: Target{Name: "s", Host: c.host, User: c.user}, Port: 22}})
		if c.ok && err != nil {
			t.Errorf("host %q user %q refused: %v", c.host, c.user, err)
		}
		if !c.ok && !errors.Is(err, ErrBadTarget) {
			t.Errorf("host %q user %q accepted (err %v)", c.host, c.user, err)
		}
	}
}

func TestValidateExtraVars(t *testing.T) {
	for _, ok := range []string{"", "user_name: alice\n", `{"a": 1}`} {
		if ValidateExtraVars(ok) != nil {
			t.Errorf("rejected %q", ok)
		}
	}
	for _, bad := range []string{"- a\n- b\n", "just text", "a: [unclosed"} {
		if ValidateExtraVars(bad) == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

// The end of a real ansible-playbook run (ansible-core 2.18).
const realOutput = `TASK [Write the file] **********************************************************
changed: [box]

PLAY RECAP *********************************************************************
box                        : ok=3    changed=1    unreachable=0    failed=0    skipped=1    rescued=0    ignored=0
web_2                      : ok=0    changed=0    unreachable=1    failed=0    skipped=0    rescued=0    ignored=0
`

func TestParseRecap(t *testing.T) {
	r := parseRecap(realOutput)
	if len(r) != 2 || r[0] != (HostRecap{Host: "box", OK: 3, Changed: 1, Skipped: 1}) || r[1].Host != "web_2" || r[1].Unreachable != 1 {
		t.Errorf("%+v", r)
	}
	if len(parseRecap("no recap here")) != 0 {
		t.Error("output without a recap should give none")
	}
}

func TestLogMasksPasswords(t *testing.T) {
	l := &logBuffer{secret: []string{"dep loy pw", ""}}
	l.Write([]byte("PANEL_PW_0=dep loy pw\nok\n"))
	if got := l.String(); got != "PANEL_PW_0=********\nok\n" {
		t.Errorf("%q", got)
	}
}

func TestPlainMarkup(t *testing.T) {
	got := plain([]any{"Set O(state=absent) to remove C(/etc/foo). See L(the docs,https://x) and M(ansible.builtin.file).", "HORIZONTALLINE"})
	if got != "Set state=absent to remove /etc/foo. See the docs and ansible.builtin.file. " {
		t.Errorf("%q", got)
	}
}
