// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package ansible

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// The module catalog: every module ansible-core ships (ansible.builtin),
// read from the ansible-doc of the very Ansible that runs the playbooks, so
// it always matches what a run can use. Verified on 2.18: 71 modules, both
// calls under a second; read once and kept.

// ModuleSummary is a module in the list.
type ModuleSummary struct {
	Name      string `json:"name"` // ansible.builtin.copy
	Short     string `json:"short"`
	CheckMode string `json:"checkMode"` // "full", "partial", "none" or "" — whether a dry run runs it
}

// ModuleOption is one option of a module.
type ModuleOption struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	Elements    string   `json:"elements,omitempty"`
	Required    bool     `json:"required"`
	Default     any      `json:"default,omitempty"`
	Choices     []any    `json:"choices,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	// Suboptions marks a dict/list option with options of its own.
	Suboptions bool `json:"suboptions,omitempty"`
}

// Module is one module's documentation.
type Module struct {
	ModuleSummary
	Description string         `json:"description"`
	Options     []ModuleOption `json:"options"`
	Notes       []string       `json:"notes,omitempty"`
	Examples    string         `json:"examples"`
	// DiffMode is whether --diff shows what it changes.
	DiffMode string `json:"diffMode"`
}

type moduleCatalog struct {
	mu      sync.Mutex
	list    []ModuleSummary
	modules map[string]*Module
}

var catalog moduleCatalog

// markupRe is Ansible's documentation markup: C(code), I(italic), B(bold),
// O(option), V(value), M(module), U(url), RV(return value), E(env var),
// P(plugin) — and L(text,url), whose text is kept.
var (
	markupRe = regexp.MustCompile(`\b(?:C|I|B|O|V|M|U|RV|E|P)\(([^()]*)\)`)
	linkRe   = regexp.MustCompile(`\bL\(([^,()]*),[^()]*\)`)
)

func plain(v any) string {
	var parts []string
	switch d := v.(type) {
	case string:
		parts = []string{d}
	case []any:
		for _, x := range d {
			if s, ok := x.(string); ok {
				parts = append(parts, s)
			}
		}
	}
	s := strings.Join(parts, " ")
	s = linkRe.ReplaceAllString(s, "$1")
	s = markupRe.ReplaceAllString(s, "$1")
	return strings.ReplaceAll(s, "HORIZONTALLINE", "")
}

// ansibleDoc runs ansible-doc with a clean environment and its own home.
func ansibleDoc(ctx context.Context, args ...string) ([]byte, error) {
	home, err := os.MkdirTemp("", "panel-ansible-doc-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(home)
	cmd := exec.CommandContext(ctx, "ansible-doc", args...)
	cmd.Env = []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "HOME=" + home, "LANG=C.UTF-8", "ANSIBLE_NOCOLOR=1"}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ansible-doc: %v %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type rawDoc struct {
	Doc struct {
		ShortDescription string                    `json:"short_description"`
		Description      any                       `json:"description"`
		Notes            []any                     `json:"notes"`
		Options          map[string]map[string]any `json:"options"`
		Attributes       map[string]struct {
			Support string `json:"support"`
		} `json:"attributes"`
	} `json:"doc"`
	Examples string `json:"examples"`
}

// load reads the catalog once; a failure isn't kept, so the next request
// tries again.
func (c *moduleCatalog) load(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.modules != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	listJSON, err := ansibleDoc(ctx, "-l", "-j", "-t", "module", "ansible.builtin")
	if err != nil {
		return err
	}
	var names map[string]string
	if err := json.Unmarshal(listJSON, &names); err != nil {
		return fmt.Errorf("reading the module list: %v", err)
	}
	all := make([]string, 0, len(names))
	for n := range names {
		all = append(all, n)
	}
	sort.Strings(all)
	docsJSON, err := ansibleDoc(ctx, append([]string{"-j", "-t", "module"}, all...)...)
	if err != nil {
		return err
	}
	var docs map[string]rawDoc
	if err := json.Unmarshal(docsJSON, &docs); err != nil {
		return fmt.Errorf("reading the module docs: %v", err)
	}
	modules := map[string]*Module{}
	var list []ModuleSummary
	for _, n := range all {
		d, ok := docs[n]
		if !ok {
			continue
		}
		m := &Module{
			ModuleSummary: ModuleSummary{Name: n, Short: plain(d.Doc.ShortDescription), CheckMode: d.Doc.Attributes["check_mode"].Support},
			Description:   plain(d.Doc.Description),
			Examples:      strings.TrimSpace(d.Examples),
			DiffMode:      d.Doc.Attributes["diff_mode"].Support,
			Options:       []ModuleOption{},
		}
		if m.Short == "" {
			m.Short = names[n]
		}
		for _, note := range d.Doc.Notes {
			if s := plain(note); s != "" {
				m.Notes = append(m.Notes, s)
			}
		}
		for name, o := range d.Doc.Options {
			opt := ModuleOption{Name: name, Description: plain(o["description"]), Default: o["default"]}
			opt.Type, _ = o["type"].(string)
			if opt.Type == "" {
				opt.Type = "str"
			}
			opt.Elements, _ = o["elements"].(string)
			opt.Required, _ = o["required"].(bool)
			opt.Choices, _ = o["choices"].([]any)
			if a, ok := o["aliases"].([]any); ok {
				for _, x := range a {
					if s, ok := x.(string); ok {
						opt.Aliases = append(opt.Aliases, s)
					}
				}
			}
			_, opt.Suboptions = o["suboptions"]
			m.Options = append(m.Options, opt)
		}
		sort.Slice(m.Options, func(i, j int) bool {
			if m.Options[i].Required != m.Options[j].Required {
				return m.Options[i].Required
			}
			return m.Options[i].Name < m.Options[j].Name
		})
		modules[n] = m
		list = append(list, m.ModuleSummary)
	}
	c.modules, c.list = modules, list
	return nil
}

// WarmModules reads the catalog in the background, so the first visitor
// doesn't wait.
func WarmModules() {
	go func() {
		if err := catalog.load(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "ansible: module catalog: %v\n", err)
		}
	}()
}

func (h *Handler) handleModules(w http.ResponseWriter, r *http.Request) {
	if err := catalog.load(r.Context()); err != nil {
		http.Error(w, "couldn't read the module list: "+err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, catalog.list, 0)
}

func (h *Handler) handleModule(w http.ResponseWriter, r *http.Request) {
	if err := catalog.load(r.Context()); err != nil {
		http.Error(w, "couldn't read the module list: "+err.Error(), http.StatusBadGateway)
		return
	}
	name := r.PathValue("name")
	if !strings.Contains(name, ".") {
		name = "ansible.builtin." + name
	}
	m, ok := catalog.modules[name]
	if !ok {
		http.Error(w, "no module "+name, http.StatusNotFound)
		return
	}
	writeJSON(w, m, 0)
}
