// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"devops-wrapper/internal/db"
	"devops-wrapper/internal/secretbox"
	"devops-wrapper/internal/servers"
	"devops-wrapper/internal/sshexec"
)

// TestAnsibleEndToEnd runs real playbooks with ansible-playbook (it must be
// installed where the test runs) on a throwaway server with Python
// (E2E_SSH_TARGET, E2E_SSH_USER — not root, with sudo by password —
// E2E_SSH_PASSWORD) and a throwaway Postgres (TEST_DATABASE_URL).
func TestAnsibleEndToEnd(t *testing.T) {
	dbURL, target, user, password := os.Getenv("TEST_DATABASE_URL"), os.Getenv("E2E_SSH_TARGET"), os.Getenv("E2E_SSH_USER"), os.Getenv("E2E_SSH_PASSWORD")
	if dbURL == "" || target == "" || user == "" || password == "" {
		t.Skip("set TEST_DATABASE_URL, E2E_SSH_TARGET, E2E_SSH_USER and E2E_SSH_PASSWORD to throwaway systems to run")
	}
	// A secret in the test's own environment, as the panel has its keys.
	os.Setenv("PANEL_TEST_SECRET", "do-not-leak-7731")
	ctx := context.Background()
	pool, err := db.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	box, _ := secretbox.New(make([]byte, 32))
	srvStore := servers.NewStore(pool)
	h := NewHandler(NewStore(pool), srvStore, box)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)
	host, portStr, _ := strings.Cut(target, ":")
	port, _ := strconv.Atoi(portStr)
	enc, _ := box.Encrypt(password)
	srv := &servers.Server{Name: "test box", Host: host, Port: port, Username: user, EncryptedPassword: enc}
	if err := srvStore.Add(ctx, srv); err != nil {
		t.Fatal(err)
	}
	do := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			json.NewEncoder(&buf).Encode(body)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, &buf))
		return rec
	}
	wait := func(id string) Run {
		t.Helper()
		var r Run
		for i := 0; i < 240; i++ {
			json.Unmarshal(do("GET", "/api/ansible/runs/"+id, nil).Body.Bytes(), &r)
			if r.Status != "running" {
				return r
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatalf("run %s still running:\n%s", id, r.Log)
		return r
	}
	start := func(pbID, mode, extra, approve string) (*httptest.ResponseRecorder, Run) {
		rec := do("POST", "/api/ansible/playbooks/"+pbID+"/runs", map[string]any{"serverIds": []string{srv.ID}, "mode": mode, "extraVars": extra, "approve": approve})
		var r Run
		json.Unmarshal(rec.Body.Bytes(), &r)
		return rec, r
	}
	ssh, err := sshexec.Dial(sshexec.Target{Host: host, Port: port, Username: user, Password: password})
	if err != nil {
		t.Fatal(err)
	}
	defer ssh.Close()
	onServer := func(cmd string) string {
		out, _, _ := ssh.Output(sshexec.AsRoot("sh -c " + sshexec.ShellQuote(cmd)))
		return strings.TrimSpace(out)
	}
	onServer("rm -f /etc/panel-ansible-test")

	// From a template: facts, dry run.
	rec := do("POST", "/api/ansible/playbooks", map[string]any{"template": "facts"})
	var facts Playbook
	json.Unmarshal(rec.Body.Bytes(), &facts)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create from template: %d %s", rec.Code, rec.Body)
	}
	_, r := start(facts.ID, "check", "", "")
	r = wait(r.ID)
	if r.Status != "success" || len(r.Recap) != 1 || r.Recap[0].Host != "test_box" || r.Recap[0].Failed != 0 {
		t.Fatalf("facts dry run: %s %s\n%s", r.Status, r.Error, r.Log)
	}
	t.Logf("facts: %s", lineWith(r.Log, "kernel"))

	// A playbook that writes a root-owned file through become.
	content := `- name: Write a file
  hosts: all
  become: true
  tasks:
    - name: Write it
      ansible.builtin.copy:
        dest: /etc/panel-ansible-test
        content: "hello {{ who }}\n"
        mode: "0644"
`
	rec = do("POST", "/api/ansible/playbooks", map[string]any{"name": "write", "files": []File{{Path: EntryPoint, Content: content}}})
	var pb Playbook
	json.Unmarshal(rec.Body.Bytes(), &pb)

	// Apply before any dry run: refused.
	if rec, _ := start(pb.ID, "apply", "who: panel", ""); rec.Code != http.StatusConflict {
		t.Errorf("apply without a dry run: %d %s", rec.Code, rec.Body)
	}
	_, r = start(pb.ID, "check", "who: panel", "")
	r = wait(r.ID)
	if r.Status != "success" || r.Recap[0].Changed != 1 || !strings.Contains(r.Log, "hello panel") {
		t.Fatalf("dry run: %s %+v\n%s", r.Status, r.Recap, r.Log)
	}
	if got := onServer("cat /etc/panel-ansible-test 2>/dev/null"); got != "" {
		t.Errorf("the dry run wrote the file: %q", got)
	}
	// Different extra vars than the dry run: still refused.
	if rec, _ := start(pb.ID, "apply", "who: other", ""); rec.Code != http.StatusConflict {
		t.Errorf("apply with other vars than the dry run: %d", rec.Code)
	}
	rec, r = start(pb.ID, "apply", "who: panel", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("apply after the dry run: %d %s", rec.Code, rec.Body)
	}
	r = wait(r.ID)
	if r.Status != "success" || r.Recap[0].Changed != 1 {
		t.Fatalf("apply: %s %+v\n%s", r.Status, r.Recap, r.Log)
	}
	if got := onServer("cat /etc/panel-ansible-test; stat -c %U /etc/panel-ansible-test"); got != "hello panel\nroot" {
		t.Errorf("on the server: %q", got)
	}
	_, r = start(pb.ID, "check", "who: panel", "")
	if r = wait(r.ID); r.Recap[0].Changed != 0 {
		t.Errorf("a second dry run should find nothing to change: %+v", r.Recap)
	}

	// A syntax error stops the run before anything happens.
	broken := []File{{Path: EntryPoint, Content: "- hosts: all\n  tasks:\n    - name: x\n      ansible.builtin.copy: [\n"}}
	do("PUT", "/api/ansible/playbooks/"+pb.ID, map[string]any{"files": broken})
	_, r = start(pb.ID, "check", "", "")
	if r = wait(r.ID); r.Status != "failed" || !strings.Contains(r.Error, "syntax error") || strings.Contains(r.Log, "== Dry run") {
		t.Errorf("syntax error: %s %q\n%s", r.Status, r.Error, r.Log)
	}

	// A localhost task doesn't see the panel's environment.
	leak := []File{{Path: EntryPoint, Content: "- hosts: localhost\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: env\n      check_mode: false\n      register: e\n    - ansible.builtin.debug:\n        var: e.stdout_lines\n"}}
	do("PUT", "/api/ansible/playbooks/"+pb.ID, map[string]any{"files": leak})
	_, r = start(pb.ID, "check", "", "")
	r = wait(r.ID)
	if strings.Contains(r.Log, "do-not-leak") || strings.Contains(r.Log, "DATABASE_URL") {
		t.Errorf("the panel's environment reached a playbook:\n%s", r.Log)
	}
	// The servers' passwords are in Ansible's own environment (it logs in
	// with them), so a local task can print them: the log masks them.
	if strings.Contains(r.Log, password) || !strings.Contains(r.Log, "PANEL_PW_0=********") {
		t.Errorf("a server password isn't masked in the log:\n%s", r.Log)
	}
	if !strings.Contains(r.Log, "ANSIBLE_CONFIG") {
		t.Errorf("the env task didn't run as expected:\n%s", r.Log)
	}

	// Upload a zipped project: its site.yml runs, with its role.
	zipped := zipOf(t, map[string]string{
		"proj/site.yml":                   "- hosts: all\n  gather_facts: false\n  roles:\n    - hello\n",
		"proj/roles/hello/tasks/main.yml": "- name: Role task\n  ansible.builtin.debug:\n    msg: \"role says hi to {{ inventory_hostname }}\"\n",
		"proj/ansible.cfg":                "[defaults]\n",
	})
	rec = do("POST", "/api/ansible/playbooks/upload", map[string]any{"filename": "proj.zip", "archive": zipped})
	var up uploadResponse
	json.Unmarshal(rec.Body.Bytes(), &up)
	if rec.Code != http.StatusCreated || up.Playbook == nil || up.Playbook.Entry != "site.yml" || up.Playbook.Name != "proj" || len(up.Notes) != 1 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body)
	}
	_, r = start(up.Playbook.ID, "check", "", "")
	if r = wait(r.ID); r.Status != "success" || !strings.Contains(r.Log, "role says hi to test_box") {
		t.Errorf("running the uploaded project: %s %s\n%s", r.Status, r.Error, r.Log)
	}

	// Cancel a long run.
	sleep := []File{{Path: EntryPoint, Content: "- hosts: all\n  gather_facts: false\n  tasks:\n    - ansible.builtin.command: sleep 120\n"}}
	do("PUT", "/api/ansible/playbooks/"+pb.ID, map[string]any{"files": sleep})
	_, r = start(pb.ID, "apply", "", "write")
	time.Sleep(4 * time.Second)
	if rec := do("POST", "/api/ansible/runs/"+r.ID+"/cancel", nil); rec.Code != http.StatusAccepted {
		t.Errorf("cancel: %d", rec.Code)
	}
	began := time.Now()
	if r = wait(r.ID); r.Status != "canceled" || time.Since(began) > 15*time.Second {
		t.Errorf("after cancel: %s in %s", r.Status, time.Since(began))
	}
}

func lineWith(s, sub string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			return strings.TrimSpace(l)
		}
	}
	return ""
}
