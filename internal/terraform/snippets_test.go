// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func templateFiles(t *testing.T, key string) []File {
	t.Helper()
	for _, tp := range Templates {
		if tp.Key == key {
			return append([]File(nil), tp.Files...)
		}
	}
	t.Fatalf("no template %s", key)
	return nil
}

// Every example, added once and then again, keeps the code meeting the
// standard: the second copy gets its own names, and the providers it uses
// are declared and pinned.
func TestSnippetsMeetTheStandard(t *testing.T) {
	for _, s := range Snippets {
		for _, base := range []string{"blank", "example"} {
			t.Run(s.Concept+"/"+base, func(t *testing.T) {
				files := templateFiles(t, base)
				for i := 0; i < 2; i++ {
					var err error
					var file string
					var line int
					files, file, line, err = AddSnippet(files, s)
					if err != nil {
						t.Fatalf("add #%d: %v", i+1, err)
					}
					content := ""
					for _, f := range files {
						if f.Path == file {
							content = f.Content
						}
					}
					if got := strings.Split(content, "\n"); line < 1 || line > len(got) || !strings.HasPrefix(got[line-1], "#") {
						t.Errorf("add #%d: line %d of %s isn't the snippet's first line", i+1, line, file)
					}
				}
				if err := ValidateFiles(files); err != nil {
					t.Fatal(err)
				}
				a := Analyze(files, AnalyzeOptions{})
				if len(a.ParseErrors) > 0 {
					t.Fatalf("parse errors: %+v", a.ParseErrors)
				}
				for _, f := range a.Errors() {
					t.Errorf("%s:%d %s %s", f.File, f.Line, f.Rule, f.Message)
				}
				for _, p := range s.Providers {
					found := false
					for _, d := range a.Providers {
						if d.Name == p.Name && d.Declared && d.Version != "" {
							found = true
						}
					}
					if !found {
						t.Errorf("provider %s isn't declared and pinned", p.Name)
					}
				}
			})
		}
	}
}

func TestAddSnippetRenamesOnClash(t *testing.T) {
	s, _ := SnippetFor("Count")
	files, _, _, err := AddSnippet(templateFiles(t, "blank"), s)
	if err != nil {
		t.Fatal(err)
	}
	files, _, _, err = AddSnippet(files, s)
	if err != nil {
		t.Fatal(err)
	}
	main := ""
	for _, f := range files {
		if f.Path == "main.tf" {
			main = f.Content
		}
	}
	for _, want := range []string{`resource "random_id" "server"`, `resource "random_id" "server_2"`, "random_id.server_2[*].hex"} {
		if !strings.Contains(main, want) {
			t.Errorf("main.tf lacks %q:\n%s", want, main)
		}
	}
}

func TestAddSnippetMakesVersionsTF(t *testing.T) {
	s, _ := SnippetFor("Resources")
	files, _, _, err := AddSnippet([]File{{Path: "main.tf", Content: "# empty\n"}}, s)
	if err != nil {
		t.Fatal(err)
	}
	var versions string
	for _, f := range files {
		if f.Path == "versions.tf" {
			versions = f.Content
		}
	}
	if !strings.Contains(versions, `source  = "hashicorp/random"`) || !strings.Contains(versions, `required_version = ">= 1.6"`) {
		t.Errorf("versions.tf:\n%s", versions)
	}
}

func TestAddSnippetRefusesCodeThatDoesntParse(t *testing.T) {
	s, _ := SnippetFor("Resources")
	if _, _, _, err := AddSnippet([]File{{Path: "main.tf", Content: "resource {"}}, s); err == nil {
		t.Error("added to code that doesn't parse")
	}
}

func TestAddProviderKeepsExistingOnes(t *testing.T) {
	files := templateFiles(t, "example")
	out, file, err := AddProvider(files, "vsphere")
	if err != nil {
		t.Fatal(err)
	}
	if file != "versions.tf" {
		t.Errorf("file = %s", file)
	}
	a := Analyze(out, AnalyzeOptions{})
	got := map[string]string{}
	for _, p := range a.Providers {
		got[p.Name] = p.Source + " " + p.Version
	}
	if got["vsphere"] != "hashicorp/vsphere ~> 2.17" || got["random"] == "" || got["null"] == "" {
		t.Errorf("providers = %v", got)
	}
	if _, _, err := AddProvider(files, "nope"); err == nil {
		t.Error("added an unknown provider")
	}
	again, _, _ := AddProvider(out, "vsphere")
	for i := range again {
		if again[i].Content != out[i].Content {
			t.Errorf("adding vsphere twice changed %s", again[i].Path)
		}
	}
}

func TestHandleSnippet(t *testing.T) {
	h := &Handler{}
	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.handleSnippet(w, httptest.NewRequest("POST", "/api/terraform/snippets", strings.NewReader(body)))
		return w
	}
	files, _ := json.Marshal(templateFiles(t, "blank"))
	w := post(`{"concept":"For_each","files":` + string(files) + `}`)
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body)
	}
	var res struct {
		Files []File `json:"files"`
		File  string `json:"file"`
		Line  int    `json:"line"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.File != "main.tf" || res.Line < 1 || len(Analyze(res.Files, AnalyzeOptions{}).Errors()) > 0 {
		t.Errorf("result = %s:%d, findings %+v", res.File, res.Line, Analyze(res.Files, AnalyzeOptions{}).Errors())
	}
	if w := post(`{"concept":"Plan","files":[]}`); w.Code != 400 {
		t.Errorf("an action concept: status %d", w.Code)
	}
	if w := post(`{"provider":"esxi","files":` + string(files) + `}`); w.Code != 200 || !strings.Contains(w.Body.String(), `josenk/esxi`) {
		t.Errorf("adding a provider: %d %s", w.Code, w.Body)
	}
	if w := post(`{"concept":"Resources","files":[{"path":"main.tf","content":"resource {"}]}`); w.Code != 422 {
		t.Errorf("code that doesn't parse: status %d", w.Code)
	}
}

// Every concept is either an example to add or an action the panel runs.
func TestEveryConceptCanBeUsed(t *testing.T) {
	actions := map[string]bool{
		"Providers": true, "State File": true, "Init": true, "Plan": true, "Apply": true, "Destroy": true,
		"Taint": true, "Import": true, "Workflows": true, "TFC/TFE": true, "Sentinel": true, "Workspace Variables": true,
	}
	for _, name := range ConceptNames() {
		_, hasSnippet := SnippetFor(name)
		if hasSnippet == actions[name] {
			t.Errorf("concept %s: snippet %v, action %v; want exactly one", name, hasSnippet, actions[name])
		}
	}
	for _, s := range Snippets {
		found := false
		for _, name := range ConceptNames() {
			found = found || name == s.Concept
		}
		if !found {
			t.Errorf("snippet for unknown concept %q", s.Concept)
		}
	}
}
