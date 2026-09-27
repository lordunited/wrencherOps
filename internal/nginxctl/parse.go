// Package nginxctl manages the configuration of an Nginx running in Docker
// on a remote server: it reads the running configuration, presents its
// server blocks and upstreams, generates new ones, and applies changes by
// testing them with "nginx -t" and reloading gracefully.
package nginxctl

import (
	"fmt"
	"strings"
)

// Directive is one Nginx directive: a name, its arguments, and for block
// directives (server, location, http, ...) the directives inside.
type Directive struct {
	Name  string
	Args  []string
	Block []*Directive // nil for a simple directive
	Line  int
}

// Parse reads Nginx configuration syntax. It's a structural parser: it
// understands quoting, comments, blocks and ${variables}, not the meaning
// of any directive, so it reads any valid Nginx config.
func Parse(src string) ([]*Directive, error) {
	p := &parser{src: src, line: 1}
	out, err := p.block(false)
	if err != nil {
		return nil, fmt.Errorf("line %d: %v", p.line, err)
	}
	return out, nil
}

type parser struct {
	src  string
	pos  int
	line int
}

// token kinds
const (
	tWord = iota
	tOpen
	tClose
	tSemi
	tEOF
)

func (p *parser) next() (kind int, text string, err error) {
	// Skip whitespace and comments.
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '\n' {
			p.line++
			p.pos++
		} else if c == ' ' || c == '\t' || c == '\r' {
			p.pos++
		} else if c == '#' {
			for p.pos < len(p.src) && p.src[p.pos] != '\n' {
				p.pos++
			}
		} else {
			break
		}
	}
	if p.pos >= len(p.src) {
		return tEOF, "", nil
	}
	switch c := p.src[p.pos]; c {
	case '{':
		p.pos++
		return tOpen, "{", nil
	case '}':
		p.pos++
		return tClose, "}", nil
	case ';':
		p.pos++
		return tSemi, ";", nil
	case '"', '\'':
		p.pos++
		var b strings.Builder
		for p.pos < len(p.src) {
			ch := p.src[p.pos]
			if ch == '\\' && p.pos+1 < len(p.src) {
				b.WriteByte(p.src[p.pos+1])
				p.pos += 2
				continue
			}
			if ch == c {
				p.pos++
				return tWord, b.String(), nil
			}
			if ch == '\n' {
				p.line++
			}
			b.WriteByte(ch)
			p.pos++
		}
		return 0, "", fmt.Errorf("unterminated quoted string")
	}
	start := p.pos
	for p.pos < len(p.src) {
		c := p.src[p.pos]
		if c == '$' && p.pos+1 < len(p.src) && p.src[p.pos+1] == '{' {
			// ${variable} is part of the word, not a block.
			end := strings.IndexByte(p.src[p.pos:], '}')
			if end < 0 {
				return 0, "", fmt.Errorf("unterminated ${")
			}
			p.pos += end + 1
			continue
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == ';' || c == '{' || c == '}' || c == '"' || c == '\'' {
			break
		}
		if c == '#' && p.pos > start {
			// A # inside a word (e.g. a URL fragment) is part of it.
			p.pos++
			continue
		}
		p.pos++
	}
	return tWord, p.src[start:p.pos], nil
}

func (p *parser) block(nested bool) ([]*Directive, error) {
	var out []*Directive
	for {
		kind, text, err := p.next()
		if err != nil {
			return nil, err
		}
		switch kind {
		case tEOF:
			if nested {
				return nil, fmt.Errorf("missing }")
			}
			return out, nil
		case tClose:
			if !nested {
				return nil, fmt.Errorf("unexpected }")
			}
			return out, nil
		case tOpen, tSemi:
			return nil, fmt.Errorf("unexpected %q", text)
		}
		d := &Directive{Name: text, Line: p.line}
		for {
			kind, text, err := p.next()
			if err != nil {
				return nil, err
			}
			if kind == tSemi {
				break
			}
			if kind == tOpen {
				if d.Block, err = p.block(true); err != nil {
					return nil, err
				}
				if d.Block == nil {
					d.Block = []*Directive{}
				}
				break
			}
			if kind == tWord {
				d.Args = append(d.Args, text)
				continue
			}
			return nil, fmt.Errorf("directive %q is missing its ;", d.Name)
		}
		out = append(out, d)
	}
}

func find(ds []*Directive, name string) *Directive {
	for _, d := range ds {
		if d.Name == name {
			return d
		}
	}
	return nil
}

func all(ds []*Directive, name string) []*Directive {
	var out []*Directive
	for _, d := range ds {
		if d.Name == name {
			out = append(out, d)
		}
	}
	return out
}

// ---- the structured view ------------------------------------------------

// Server is one server block.
type Server struct {
	File          string     `json:"file"`
	Line          int        `json:"line"`
	Context       string     `json:"context"` // "http" or "stream"
	Names         []string   `json:"names"`
	Listen        []string   `json:"listen"`
	DefaultServer bool       `json:"defaultServer"`
	SSL           bool       `json:"ssl"`
	Certificate   string     `json:"certificate,omitempty"`
	Return        string     `json:"return,omitempty"` // server-level return, e.g. a redirect
	Locations     []Location `json:"locations"`
	ProxyPass     string     `json:"proxyPass,omitempty"` // stream servers
}

// Location is one location block inside a server.
type Location struct {
	Match     string `json:"match"` // e.g. "/", "= /healthz", "~ \.php$"
	ProxyPass string `json:"proxyPass,omitempty"`
	Return    string `json:"return,omitempty"`
	Root      string `json:"root,omitempty"`
	Alias     string `json:"alias,omitempty"`
	TryFiles  string `json:"tryFiles,omitempty"`
}

// Upstream is one upstream group of backends.
type Upstream struct {
	File      string   `json:"file"`
	Line      int      `json:"line"`
	Context   string   `json:"context"`
	Name      string   `json:"name"`
	Method    string   `json:"method"` // "round robin", "least_conn", "ip_hash", "hash ..."
	Servers   []string `json:"servers"`
	Keepalive string   `json:"keepalive,omitempty"`
}

// Globals are the settings people most often need to look at.
type Globals struct {
	WorkerProcesses   string   `json:"workerProcesses,omitempty"`
	WorkerConnections string   `json:"workerConnections,omitempty"`
	ClientMaxBodySize string   `json:"clientMaxBodySize,omitempty"`
	KeepaliveTimeout  string   `json:"keepaliveTimeout,omitempty"`
	ServerTokens      string   `json:"serverTokens,omitempty"`
	Gzip              string   `json:"gzip,omitempty"`
	Sendfile          string   `json:"sendfile,omitempty"`
	ErrorLog          string   `json:"errorLog,omitempty"`
	LogFormats        []string `json:"logFormats,omitempty"`
	Includes          []string `json:"includes,omitempty"`
}

// Overview is the structured view of a whole configuration.
type Overview struct {
	Globals   Globals    `json:"globals"`
	Servers   []Server   `json:"servers"`
	Upstreams []Upstream `json:"upstreams"`
	// ParseErrors lists files the panel couldn't read structurally; they
	// still show as files and can be edited.
	ParseErrors []string `json:"parseErrors,omitempty"`
}

// Build parses each file and collects servers, upstreams and globals.
// mainPath is the top-level nginx.conf; included files are fragments that
// start in the context of the include (normally http).
func Build(files map[string]string, mainPath string) Overview {
	ov := Overview{Servers: []Server{}, Upstreams: []Upstream{}}
	for path, content := range files {
		ds, err := Parse(content)
		if err != nil {
			ov.ParseErrors = append(ov.ParseErrors, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		ctx := "http"
		if path == mainPath {
			ctx = "main"
			collectGlobals(ds, &ov.Globals)
		}
		walk(ds, path, ctx, &ov)
	}
	sortOverview(&ov)
	return ov
}

func joinArgs(d *Directive) string {
	if d == nil {
		return ""
	}
	return strings.Join(d.Args, " ")
}

func collectGlobals(ds []*Directive, g *Globals) {
	g.WorkerProcesses = joinArgs(find(ds, "worker_processes"))
	g.ErrorLog = joinArgs(find(ds, "error_log"))
	if ev := find(ds, "events"); ev != nil {
		g.WorkerConnections = joinArgs(find(ev.Block, "worker_connections"))
	}
	if h := find(ds, "http"); h != nil {
		g.ClientMaxBodySize = joinArgs(find(h.Block, "client_max_body_size"))
		g.KeepaliveTimeout = joinArgs(find(h.Block, "keepalive_timeout"))
		g.ServerTokens = joinArgs(find(h.Block, "server_tokens"))
		g.Gzip = joinArgs(find(h.Block, "gzip"))
		g.Sendfile = joinArgs(find(h.Block, "sendfile"))
		for _, lf := range all(h.Block, "log_format") {
			if len(lf.Args) > 0 {
				g.LogFormats = append(g.LogFormats, lf.Args[0])
			}
		}
		for _, inc := range all(h.Block, "include") {
			g.Includes = append(g.Includes, joinArgs(inc))
		}
	}
}

func walk(ds []*Directive, file, ctx string, ov *Overview) {
	for _, d := range ds {
		switch {
		case d.Name == "http" && d.Block != nil:
			walk(d.Block, file, "http", ov)
		case d.Name == "stream" && d.Block != nil:
			walk(d.Block, file, "stream", ov)
		case d.Name == "server" && d.Block != nil:
			ov.Servers = append(ov.Servers, readServer(d, file, ctx))
		case d.Name == "upstream" && d.Block != nil && len(d.Args) > 0:
			ov.Upstreams = append(ov.Upstreams, readUpstream(d, file, ctx))
		}
	}
}

func readServer(d *Directive, file, ctx string) Server {
	if ctx == "main" {
		ctx = "http"
	}
	s := Server{File: file, Line: d.Line, Context: ctx, Names: []string{}, Listen: []string{}, Locations: []Location{}}
	for _, l := range all(d.Block, "listen") {
		s.Listen = append(s.Listen, joinArgs(l))
		for _, a := range l.Args[1:] {
			switch a {
			case "ssl":
				s.SSL = true
			case "default_server", "default":
				s.DefaultServer = true
			}
		}
	}
	for _, n := range all(d.Block, "server_name") {
		s.Names = append(s.Names, n.Args...)
	}
	if c := find(d.Block, "ssl_certificate"); c != nil {
		s.SSL = true
		s.Certificate = joinArgs(c)
	}
	s.Return = joinArgs(find(d.Block, "return"))
	s.ProxyPass = joinArgs(find(d.Block, "proxy_pass"))
	var locs func(ds []*Directive, prefix string)
	locs = func(ds []*Directive, prefix string) {
		for _, l := range all(ds, "location") {
			loc := Location{
				Match:     strings.TrimSpace(prefix + joinArgs(l)),
				ProxyPass: joinArgs(find(l.Block, "proxy_pass")),
				Return:    joinArgs(find(l.Block, "return")),
				Root:      joinArgs(find(l.Block, "root")),
				Alias:     joinArgs(find(l.Block, "alias")),
				TryFiles:  joinArgs(find(l.Block, "try_files")),
			}
			if loc.ProxyPass == "" {
				loc.ProxyPass = joinArgs(find(l.Block, "fastcgi_pass"))
			}
			if loc.ProxyPass == "" {
				loc.ProxyPass = joinArgs(find(l.Block, "grpc_pass"))
			}
			s.Locations = append(s.Locations, loc)
			locs(l.Block, loc.Match+" › ")
		}
	}
	locs(d.Block, "")
	return s
}

func readUpstream(d *Directive, file, ctx string) Upstream {
	if ctx == "main" {
		ctx = "http"
	}
	u := Upstream{File: file, Line: d.Line, Context: ctx, Name: d.Args[0], Method: "round robin", Servers: []string{}}
	for _, x := range d.Block {
		switch x.Name {
		case "server":
			u.Servers = append(u.Servers, joinArgs(x))
		case "least_conn", "ip_hash", "random", "least_time":
			u.Method = strings.TrimSpace(x.Name + " " + joinArgs(x))
		case "hash":
			u.Method = "hash " + joinArgs(x)
		case "keepalive":
			u.Keepalive = joinArgs(x)
		}
	}
	return u
}
