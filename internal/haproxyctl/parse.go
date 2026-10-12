// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package haproxyctl

import (
	"strings"

	"devops-wrapper/internal/dockerx"
)

// section is one global/defaults/frontend/backend/listen/... block. HAProxy's
// grammar has no braces: a section runs from its header line to the next
// one (or EOF), so this is deliberately much simpler than a real parser —
// it tracks the directives the panel shows or acts on, not all of them.
type section struct {
	kind string
	name string
	file string // "" when parsed from text that isn't one file
	line int    // the header's line, 1-based

	binds          []string // addresses, e.g. "*:80"
	tlsBinds       []string // the subset of binds that terminate TLS ("ssl")
	certs          []string // "crt" paths on any bind
	mode           string   // this section's own mode; see effectiveModes
	balance        string
	servers        []string
	defaultBackend string
	useBackends    []string // "backend [if|unless cond]"
	monitorURI     string
	statsEnabled   bool
	statsURI       string
	healthCheck    string              // e.g. "httpchk GET /health": how servers are checked beyond a TCP connect
	redirect       string              // "http-request redirect" arguments, when the section redirects
	settings       []Setting           // global and defaults sections only: every directive, in order
	hostACLs       map[string][]string // acl name -> Host header (or SNI) values it matches
}

// Setting is one directive of a global or defaults section.
type Setting struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Route is one way a frontend picks a backend: a use_backend rule — with
// the Host header values its condition matches when it's a host rule — or
// the default_backend, for everything no rule matched.
type Route struct {
	Backend   string   `json:"backend"`
	Hosts     []string `json:"hosts,omitempty"`
	Condition string   `json:"condition,omitempty"` // the raw "if ..." / "unless ..." clause
	Default   bool     `json:"default,omitempty"`
}

func (s section) describe() string {
	if s.name == "" {
		return s.kind
	}
	return s.kind + " " + s.name
}

// routes lists s's use_backend rules in order, then its default_backend.
// A rule's hosts come from host-matching ACLs its "if" condition uses,
// named or inline ("{ hdr(host) -i a.com }"); negated ACLs and "unless"
// conditions match everything except those hosts, so they list none.
func (s section) routes() []Route {
	var out []Route
	for _, ub := range s.useBackends {
		f := strings.Fields(ub)
		r := Route{Backend: f[0]}
		if len(f) > 1 {
			r.Condition = strings.Join(f[1:], " ")
			r.Hosts = condHosts(f[1:], s.hostACLs)
		}
		out = append(out, r)
	}
	if s.defaultBackend != "" {
		out = append(out, Route{Backend: s.defaultBackend, Default: true})
	}
	return out
}

func condHosts(cond []string, acls map[string][]string) []string {
	if len(cond) == 0 || cond[0] != "if" {
		return nil
	}
	seen := map[string]bool{}
	var hosts []string
	add := func(hs []string) {
		for _, h := range hs {
			if !seen[h] {
				seen[h] = true
				hosts = append(hosts, h)
			}
		}
	}
	for i := 1; i < len(cond); i++ {
		t := cond[i]
		if t == "{" {
			j := i + 1
			for j < len(cond) && cond[j] != "}" {
				j++
			}
			if inner := cond[i+1 : j]; len(inner) > 1 && isHostCriterion(inner[0]) {
				add(aclValues(inner[1:]))
			}
			i = j
			continue
		}
		if hs, ok := acls[t]; ok {
			add(hs)
		}
	}
	return hosts
}

// isHostCriterion reports whether an ACL fetch reads the requested host:
// the Host header in any of its spellings (hdr(host), req.hdr(host),
// hdr_dom(host), "...,field(1,:)"), or the TLS SNI name.
func isHostCriterion(c string) bool {
	c = strings.ToLower(c)
	return (strings.Contains(c, "hdr") && strings.Contains(c, "(host)")) || strings.Contains(c, "sni")
}

// aclValues returns the patterns of an ACL, skipping its flags ("-i",
// "-m str", "-f file", ...).
func aclValues(tokens []string) []string {
	var out []string
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "-f", "-m", "-M", "-u":
			i++
			continue
		}
		if strings.HasPrefix(t, "-") {
			continue
		}
		out = append(out, strings.ToLower(t))
	}
	return out
}

// SectionView is one config section, as shown to the panel.
type SectionView struct {
	Kind           string    `json:"kind"`
	Name           string    `json:"name,omitempty"`
	File           string    `json:"file,omitempty"`
	Line           int       `json:"line,omitempty"`
	Binds          []string  `json:"binds,omitempty"`
	TLS            bool      `json:"tls,omitempty"`
	Certificates   []string  `json:"certificates,omitempty"`
	Mode           string    `json:"mode,omitempty"` // effective: its own, else inherited from defaults
	Balance        string    `json:"balance,omitempty"`
	Servers        []string  `json:"servers,omitempty"`
	DefaultBackend string    `json:"defaultBackend,omitempty"`
	UseBackends    []string  `json:"useBackends,omitempty"`
	Routes         []Route   `json:"routes,omitempty"`
	MonitorURI     string    `json:"monitorUri,omitempty"`
	StatsEnabled   bool      `json:"statsEnabled,omitempty"`
	StatsURI       string    `json:"statsUri,omitempty"`
	HealthCheck    string    `json:"healthCheck,omitempty"`
	Redirect       string    `json:"redirect,omitempty"`
	Settings       []Setting `json:"settings,omitempty"`
}

func (s section) view(mode string) SectionView {
	return SectionView{
		Kind: s.kind, Name: s.name, File: s.file, Line: s.line,
		Binds: s.binds, TLS: len(s.tlsBinds) > 0, Certificates: s.certs, Mode: mode,
		Balance: s.balance, Servers: s.servers, DefaultBackend: s.defaultBackend,
		UseBackends: s.useBackends, Routes: s.routes(),
		MonitorURI: s.monitorURI, StatsEnabled: s.statsEnabled, StatsURI: s.statsURI,
		HealthCheck: s.healthCheck, Redirect: s.redirect, Settings: s.settings,
	}
}

var sectionKeywords = map[string]bool{
	"global": true, "defaults": true, "frontend": true, "backend": true,
	"listen": true, "resolvers": true, "mailers": true, "peers": true,
	"program": true, "ring": true, "cache": true, "http-errors": true,
	"userlist": true, "log-forward": true,
}

// isSectionHeader reports whether a raw config line starts a section — the
// same test parseFile uses, for code that edits the raw lines.
func isSectionHeader(line string) bool {
	f := strings.Fields(line)
	return len(f) > 0 && sectionKeywords[f[0]]
}

// parseSections splits a config (or several concatenated together) into
// its sections.
func parseSections(cfg string) []section {
	return parseFile("", cfg)
}

// parseFiles parses each file in order into one list of sections, each
// recording the file it came from — the order sections apply in, since a
// "defaults" section affects everything after it, across files too.
func parseFiles(files []dockerx.File) []section {
	var out []section
	for _, f := range files {
		out = append(out, parseFile(f.Path, f.Content)...)
	}
	return out
}

// parseFile splits one config file into its sections, recording each
// one's file and header line.
func parseFile(path, content string) []section {
	var sections []section
	var cur section
	haveCur := false
	flush := func() {
		if haveCur {
			sections = append(sections, cur)
		}
	}
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if sectionKeywords[fields[0]] {
			flush()
			cur = section{kind: fields[0], file: path, line: i + 1, hostACLs: map[string][]string{}}
			if len(fields) > 1 {
				cur.name = fields[1]
			}
			haveCur = true
			continue
		}
		if !haveCur {
			continue
		}
		if cur.kind == "global" || cur.kind == "defaults" {
			name, rest := fields[0], fields[1:]
			if (name == "timeout" || name == "stats") && len(rest) > 0 {
				name, rest = name+" "+rest[0], rest[1:]
			}
			cur.settings = append(cur.settings, Setting{Name: name, Value: strings.Join(rest, " ")})
		}
		arg := func(i int) string {
			if i < len(fields) {
				return fields[i]
			}
			return ""
		}
		switch fields[0] {
		case "bind":
			if addr := arg(1); addr != "" {
				cur.binds = append(cur.binds, addr)
				for j := 2; j < len(fields); j++ {
					switch fields[j] {
					case "ssl":
						cur.tlsBinds = append(cur.tlsBinds, addr)
					case "crt":
						if c := arg(j + 1); c != "" {
							cur.certs = append(cur.certs, c)
						}
					}
				}
			}
		case "mode":
			cur.mode = arg(1)
		case "balance":
			cur.balance = strings.Join(fields[1:], " ")
		case "server":
			if len(fields) > 1 {
				cur.servers = append(cur.servers, strings.Join(fields[1:], " "))
			}
		case "default_backend":
			cur.defaultBackend = arg(1)
		case "use_backend":
			if len(fields) > 1 {
				cur.useBackends = append(cur.useBackends, strings.Join(fields[1:], " "))
			}
		case "acl":
			if len(fields) > 3 && isHostCriterion(fields[2]) {
				cur.hostACLs[fields[1]] = append(cur.hostACLs[fields[1]], aclValues(fields[3:])...)
			}
		case "monitor-uri":
			cur.monitorURI = arg(1)
		case "stats":
			if arg(1) == "enable" {
				cur.statsEnabled = true
			}
			if arg(1) == "uri" {
				cur.statsURI = arg(2)
			}
		case "option":
			if o := arg(1); strings.HasSuffix(o, "chk") || strings.HasSuffix(o, "check") {
				cur.healthCheck = strings.Join(fields[1:], " ")
			}
		case "http-request":
			if arg(1) == "redirect" {
				cur.redirect = strings.Join(fields[2:], " ")
			}
		}
	}
	flush()
	return sections
}

// effectiveModes returns the mode each section actually runs in: its own
// "mode", else the one of the nearest defaults section above it, else
// "tcp" — HAProxy's own default. Verified against a real container that
// with no mode anywhere, HTTP-only directives ("stats", "monitor-uri")
// are ignored with just a warning, so treating unset as HTTP would be
// wrong in exactly the way that matters.
func effectiveModes(sections []section) []string {
	modes := make([]string, len(sections))
	defaultsMode := ""
	for i, s := range sections {
		if s.kind == "defaults" {
			defaultsMode = s.mode
		}
		m := s.mode
		if m == "" {
			m = defaultsMode
		}
		if m == "" {
			m = "tcp"
		}
		modes[i] = m
	}
	return modes
}
