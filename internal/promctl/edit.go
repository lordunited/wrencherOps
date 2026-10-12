// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package promctl

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Edits work on yaml.v3's node tree rather than decoding into structs, so
// the comments in hand-written configs ("#vpn node exporter") survive. The
// file is re-indented to two spaces on write; content is otherwise kept.

// ConfiguredJob is a scrape job as written in the config file.
type ConfiguredJob struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`               // "exporter" or "blackbox"
	Module   string `json:"module,omitempty"`   // blackbox module, e.g. http_2xx
	Exporter string `json:"exporter,omitempty"` // blackbox exporter host:port
}

// ConfigJobs lists the scrape jobs in a config file.
func ConfigJobs(config string) []ConfiguredJob {
	var parsed struct {
		ScrapeConfigs []struct {
			JobName     string              `yaml:"job_name"`
			MetricsPath string              `yaml:"metrics_path"`
			Params      map[string][]string `yaml:"params"`
			Relabel     []struct {
				TargetLabel string `yaml:"target_label"`
				Replacement string `yaml:"replacement"`
			} `yaml:"relabel_configs"`
		} `yaml:"scrape_configs"`
	}
	if yaml.Unmarshal([]byte(config), &parsed) != nil {
		return nil
	}
	var jobs []ConfiguredJob
	for _, sc := range parsed.ScrapeConfigs {
		j := ConfiguredJob{Name: sc.JobName, Kind: "exporter"}
		if sc.MetricsPath == "/probe" {
			j.Kind = "blackbox"
			if m := sc.Params["module"]; len(m) > 0 {
				j.Module = m[0]
			}
			for _, r := range sc.Relabel {
				if r.TargetLabel == "__address__" && r.Replacement != "" {
					j.Exporter = r.Replacement
				}
			}
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// TargetSpec is one address to scrape or probe, with an optional note that
// is written as a comment next to it.
type TargetSpec struct {
	Address string `json:"address"`
	Comment string `json:"comment"`
}

// JobSpec describes a new scrape job.
type JobSpec struct {
	Name           string       `json:"name"`
	Kind           string       `json:"kind"` // "exporter", "blackbox-http", "blackbox-tcp" or "blackbox-ssl"
	Module         string       `json:"module"`
	ExporterAddr   string       `json:"exporterAddress"` // blackbox exporter host:port
	ScrapeInterval string       `json:"scrapeInterval"`
	Targets        []TargetSpec `json:"targets"`
}

// RuleSpec describes a new alerting rule.
type RuleSpec struct {
	Alert       string            `json:"alert"`
	Expr        string            `json:"expr"`
	For         string            `json:"for"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

var (
	nameRe     = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_:-]*$`)
	durationRe = regexp.MustCompile(`^([0-9]+(ms|s|m|h|d|w|y))+$`)
)

func parseDoc(content string) (*yaml.Node, *yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, nil, fmt.Errorf("the file is not valid YAML: %v", err)
	}
	if doc.Kind == 0 {
		// Empty file.
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, nil, fmt.Errorf("the file's top level is not a mapping")
	}
	return &doc, root, nil
}

func encodeDoc(doc *yaml.Node) (string, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapSet(m *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = value
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func scalar(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func toNode(v any) (*yaml.Node, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return &n, nil
}

func targetNode(t TargetSpec) *yaml.Node {
	n := scalar(strings.TrimSpace(t.Address))
	if c := strings.TrimSpace(t.Comment); c != "" {
		n.LineComment = "# " + c
	}
	return n
}

func cleanTargets(targets []TargetSpec) ([]TargetSpec, error) {
	var out []TargetSpec
	for _, t := range targets {
		t.Address = strings.TrimSpace(t.Address)
		if t.Address == "" {
			continue
		}
		if strings.ContainsAny(t.Address, " \t#") {
			return nil, fmt.Errorf("%q is not a valid target address", t.Address)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("add at least one target")
	}
	return out, nil
}

func findJob(root *yaml.Node, name string) *yaml.Node {
	scrape := mapGet(root, "scrape_configs")
	if scrape == nil || scrape.Kind != yaml.SequenceNode {
		return nil
	}
	for _, job := range scrape.Content {
		if n := mapGet(job, "job_name"); n != nil && n.Value == name {
			return job
		}
	}
	return nil
}

// AddTargets appends targets to an existing job's first static_configs
// block. Addresses the job already has are skipped and reported.
func AddTargets(config, job string, targets []TargetSpec) (updated string, skipped []string, err error) {
	targets, err = cleanTargets(targets)
	if err != nil {
		return "", nil, err
	}
	doc, root, err := parseDoc(config)
	if err != nil {
		return "", nil, err
	}
	jobNode := findJob(root, job)
	if jobNode == nil {
		return "", nil, fmt.Errorf("job %q is not in the config", job)
	}

	static := mapGet(jobNode, "static_configs")
	if static == nil || static.Kind != yaml.SequenceNode {
		static = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(jobNode, "static_configs", static)
	}
	existing := map[string]bool{}
	for _, sc := range static.Content {
		if list := mapGet(sc, "targets"); list != nil {
			for _, t := range list.Content {
				existing[t.Value] = true
			}
		}
	}
	if len(static.Content) == 0 {
		static.Content = append(static.Content, &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"})
	}
	first := static.Content[0]
	list := mapGet(first, "targets")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(first, "targets", list)
	}
	list.Style = 0 // block style, one target per line

	added := 0
	for _, t := range targets {
		if existing[t.Address] {
			skipped = append(skipped, t.Address)
			continue
		}
		existing[t.Address] = true
		list.Content = append(list.Content, targetNode(t))
		added++
	}
	if added == 0 {
		return "", skipped, fmt.Errorf("every target is already in job %q", job)
	}
	updated, err = encodeDoc(doc)
	return updated, skipped, err
}

var blackboxModules = map[string]string{
	"blackbox-http": "http_2xx",
	"blackbox-tcp":  "tcp_connect",
	"blackbox-ssl":  "http_2xx",
}

// AddJob appends a new scrape job. Blackbox kinds get the standard probe
// wiring: /probe, the module parameter, and relabeling that sends each
// target to the blackbox exporter as its ?target= parameter.
func AddJob(config string, spec JobSpec) (string, error) {
	spec.Name = strings.TrimSpace(spec.Name)
	if !nameRe.MatchString(spec.Name) {
		return "", fmt.Errorf("job name %q may only use letters, digits, _ : and -", spec.Name)
	}
	if spec.ScrapeInterval != "" && !durationRe.MatchString(spec.ScrapeInterval) {
		return "", fmt.Errorf("scrape interval %q is not a duration like 15s or 1m", spec.ScrapeInterval)
	}
	targets, err := cleanTargets(spec.Targets)
	if err != nil {
		return "", err
	}
	doc, root, err := parseDoc(config)
	if err != nil {
		return "", err
	}
	if findJob(root, spec.Name) != nil {
		return "", fmt.Errorf("a job named %q already exists; add targets to it instead", spec.Name)
	}

	job := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapSet(job, "job_name", scalar(spec.Name))
	if spec.ScrapeInterval != "" {
		mapSet(job, "scrape_interval", scalar(spec.ScrapeInterval))
	}

	blackbox := strings.HasPrefix(spec.Kind, "blackbox")
	if blackbox {
		module := strings.TrimSpace(spec.Module)
		if module == "" {
			module = blackboxModules[spec.Kind]
		}
		if module == "" {
			return "", fmt.Errorf("unknown job kind %q", spec.Kind)
		}
		exporter := strings.TrimSpace(spec.ExporterAddr)
		if exporter == "" || strings.ContainsAny(exporter, " \t/") {
			return "", fmt.Errorf("enter the blackbox exporter address as host:port, e.g. 172.22.241.41:9115")
		}
		mapSet(job, "metrics_path", scalar("/probe"))
		params, err := toNode(map[string][]string{"module": {module}})
		if err != nil {
			return "", err
		}
		mapSet(job, "params", params)
	} else if spec.Kind != "" && spec.Kind != "exporter" {
		return "", fmt.Errorf("unknown job kind %q", spec.Kind)
	}

	list := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, t := range targets {
		list.Content = append(list.Content, targetNode(t))
	}
	sc := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapSet(sc, "targets", list)
	mapSet(job, "static_configs", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Content: []*yaml.Node{sc}})

	scrape := mapGet(root, "scrape_configs")
	if scrape == nil || scrape.Kind != yaml.SequenceNode {
		scrape = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(root, "scrape_configs", scrape)
	}

	if blackbox {
		relabel, err := toNode([]map[string]any{
			{"source_labels": []string{"__address__"}, "target_label": "__param_target"},
			{"source_labels": []string{"__param_target"}, "target_label": "instance"},
			{"target_label": "__address__", "replacement": strings.TrimSpace(spec.ExporterAddr)},
		})
		if err != nil {
			return "", err
		}
		mapSet(job, "relabel_configs", relabel)
	}
	scrape.Content = append(scrape.Content, job)
	return encodeDoc(doc)
}

// EnsureRuleFile adds entry to the config's rule_files list if it isn't
// there yet. changed reports whether the config was modified.
func EnsureRuleFile(config, entry string) (updated string, changed bool, err error) {
	doc, root, err := parseDoc(config)
	if err != nil {
		return "", false, err
	}
	list := mapGet(root, "rule_files")
	if list == nil || list.Kind != yaml.SequenceNode {
		list = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(root, "rule_files", list)
	}
	for _, n := range list.Content {
		if n.Value == entry {
			return config, false, nil
		}
	}
	list.Content = append(list.Content, scalar(entry))
	updated, err = encodeDoc(doc)
	return updated, true, err
}

// AddRule adds an alerting rule to group in a rule file, creating the group
// (and the file's groups list) when missing.
func AddRule(ruleFile, group string, rule RuleSpec) (string, error) {
	rule.Alert = strings.TrimSpace(rule.Alert)
	rule.Expr = strings.TrimSpace(rule.Expr)
	group = strings.TrimSpace(group)
	if !nameRe.MatchString(rule.Alert) {
		return "", fmt.Errorf("alert name %q may only use letters, digits, _ : and -", rule.Alert)
	}
	if rule.Expr == "" {
		return "", fmt.Errorf("enter the PromQL expression that should trigger the alert")
	}
	if rule.For != "" && !durationRe.MatchString(rule.For) {
		return "", fmt.Errorf("%q is not a duration like 5m or 1h", rule.For)
	}
	if group == "" {
		return "", fmt.Errorf("enter a group name")
	}

	doc, root, err := parseDoc(ruleFile)
	if err != nil {
		return "", err
	}
	groups := mapGet(root, "groups")
	if groups == nil || groups.Kind != yaml.SequenceNode {
		groups = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(root, "groups", groups)
	}
	var g *yaml.Node
	for _, candidate := range groups.Content {
		if n := mapGet(candidate, "name"); n != nil && n.Value == group {
			g = candidate
			break
		}
	}
	if g == nil {
		g = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		mapSet(g, "name", scalar(group))
		mapSet(g, "rules", &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"})
		groups.Content = append(groups.Content, g)
	}
	rules := mapGet(g, "rules")
	if rules == nil || rules.Kind != yaml.SequenceNode {
		rules = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(g, "rules", rules)
	}
	for _, r := range rules.Content {
		if n := mapGet(r, "alert"); n != nil && n.Value == rule.Alert {
			return "", fmt.Errorf("group %q already has an alert named %q", group, rule.Alert)
		}
	}

	r := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	mapSet(r, "alert", scalar(rule.Alert))
	expr := scalar(rule.Expr)
	if strings.Contains(rule.Expr, "\n") {
		expr.Style = yaml.LiteralStyle
	}
	mapSet(r, "expr", expr)
	if rule.For != "" {
		mapSet(r, "for", scalar(rule.For))
	}
	if labels := nonEmpty(rule.Labels); len(labels) > 0 {
		n, err := toNode(labels)
		if err != nil {
			return "", err
		}
		mapSet(r, "labels", n)
	}
	if ann := nonEmpty(rule.Annotations); len(ann) > 0 {
		n, err := toNode(ann)
		if err != nil {
			return "", err
		}
		mapSet(r, "annotations", n)
	}
	rules.Content = append(rules.Content, r)
	return encodeDoc(doc)
}

func nonEmpty(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// TargetNotes reads the comment written next to each static target in a
// config ("- 10.0.0.5:9100 #vpn node exporter"), keyed by job then target
// address. Hand-kept configs often name their servers this way, and
// Prometheus itself never reports comments.
func TargetNotes(config string) map[string]map[string]string {
	notes := map[string]map[string]string{}
	_, root, err := parseDoc(config)
	if err != nil {
		return notes
	}
	scrape := mapGet(root, "scrape_configs")
	if scrape == nil {
		return notes
	}
	for _, job := range scrape.Content {
		name := mapGet(job, "job_name")
		static := mapGet(job, "static_configs")
		if name == nil || static == nil {
			continue
		}
		for _, sc := range static.Content {
			list := mapGet(sc, "targets")
			if list == nil {
				continue
			}
			for _, t := range list.Content {
				note := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(t.LineComment), "#"))
				if note == "" {
					continue
				}
				if notes[name.Value] == nil {
					notes[name.Value] = map[string]string{}
				}
				notes[name.Value][t.Value] = note
			}
		}
	}
	return notes
}
