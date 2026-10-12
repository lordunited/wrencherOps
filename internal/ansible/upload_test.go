// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"strings"
	"testing"
)

const sitePlay = "- hosts: all\n  roles:\n    - web\n"

// project is a typical Ansible project, as it'd be zipped from its folder.
var project = map[string]string{
	"myproj/site.yml":                 sitePlay,
	"myproj/roles/web/tasks/main.yml": "- ansible.builtin.debug: msg=hi\n",
	"myproj/group_vars/all.yml":       "x: 1\n",
	"myproj/ansible.cfg":              "[defaults]\n",
	"myproj/inventory":                "web1\n",
	"myproj/.git/config":              "[core]\n",
	"myproj/files/logo.png":           "\x89PNG\r\n\x1a\n\x00\x00",
	"myproj/notes with space.txt":     "hi\n",
	"myproj/../escape.yml":            "- hosts: all\n",
	"__MACOSX/myproj/._site.yml":      "junk",
}

func zipOf(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(content))
	}
	zw.Close()
	return buf.Bytes()
}

func tgzOf(t *testing.T, files map[string]string, link bool) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(content)), Typeflag: tar.TypeReg})
		tw.Write([]byte(content))
	}
	if link {
		tw.WriteHeader(&tar.Header{Name: "myproj/secrets.yml", Linkname: "/etc/shadow", Typeflag: tar.TypeSymlink})
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func checkProject(t *testing.T, kind string, files []File, notes []string) {
	t.Helper()
	got := map[string]bool{}
	for _, f := range files {
		got[f.Path] = true
	}
	for _, want := range []string{"site.yml", "roles/web/tasks/main.yml", "group_vars/all.yml"} {
		if !got[want] {
			t.Errorf("%s: %s missing from %v", kind, want, files)
		}
	}
	if len(files) != 3 {
		t.Errorf("%s: kept %d files, want 3: %v", kind, len(files), files)
	}
	all := strings.Join(notes, "\n")
	for _, want := range []string{"ansible.cfg: the panel builds", "inventory: the panel builds", "logo.png: not a text file", "notes with space.txt: has characters", "outside the folder"} {
		if !strings.Contains(all, want) {
			t.Errorf("%s: no note %q in:\n%s", kind, want, all)
		}
	}
	if strings.Contains(all, ".git") || strings.Contains(all, "__MACOSX") {
		t.Errorf("%s: clutter should be dropped silently:\n%s", kind, all)
	}
	entry, _, err := DetectEntry(files)
	if err != nil || entry != "site.yml" {
		t.Errorf("%s: entry %q %v", kind, entry, err)
	}
	if err := ValidateFiles(files, entry); err != nil {
		t.Errorf("%s: the result doesn't validate: %v", kind, err)
	}
}

func TestExtractZip(t *testing.T) {
	files, notes, err := ExtractArchive("myproj.zip", zipOf(t, project))
	if err != nil {
		t.Fatal(err)
	}
	checkProject(t, "zip", files, notes)
}

func TestExtractTarGz(t *testing.T) {
	files, notes, err := ExtractArchive("myproj.tar.gz", tgzOf(t, project, true))
	if err != nil {
		t.Fatal(err)
	}
	checkProject(t, "tar.gz", files, notes)
	if !strings.Contains(strings.Join(notes, "\n"), "secrets.yml: not a regular file") {
		t.Errorf("a symlink should be left out with a note: %v", notes)
	}
}

func TestExtractRefuses(t *testing.T) {
	if _, _, err := ExtractArchive("x.rar", []byte("x")); err == nil {
		t.Error("an unknown format was accepted")
	}
	if _, _, err := ExtractArchive("x.zip", []byte("not a zip")); err == nil {
		t.Error("a broken zip was accepted")
	}
	if _, _, err := ExtractArchive("x.zip", make([]byte, maxArchive+1)); err == nil {
		t.Error("an oversized archive was accepted")
	}
}

func TestDetectEntry(t *testing.T) {
	two := []File{{Path: "a.yml", Content: "- hosts: web\n"}, {Path: "b.yml", Content: "- hosts: db\n"}, {Path: "vars.yml", Content: "x: 1\n"}}
	if _, c, err := DetectEntry(two); !errors.Is(err, errAmbiguousEntry) || strings.Join(c, ",") != "a.yml,b.yml" {
		t.Errorf("two plays: %v %v", c, err)
	}
	if e, _, _ := DetectEntry(append(two, File{Path: "main.yml", Content: "- import_playbook: a.yml\n"})); e != "main.yml" {
		t.Errorf("main.yml should win: %q", e)
	}
	if e, _, _ := DetectEntry([]File{{Path: "deploy.yaml", Content: "- hosts: all\n  tasks: []\n"}, {Path: "vars.yml", Content: "x: 1\n"}}); e != "deploy.yaml" {
		t.Errorf("the only play: %q", e)
	}
	if _, _, err := DetectEntry([]File{{Path: "vars.yml", Content: "x: 1\n"}}); err == nil {
		t.Error("no play at all should be an error")
	}
}
