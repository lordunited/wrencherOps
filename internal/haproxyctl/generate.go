// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package haproxyctl

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"devops-wrapper/internal/dockerx"
)

// SiteSpec describes a new site the panel adds to an HAProxy config: a
// backend of its own, plus a host rule (or the default route) on the
// frontend that serves its port. Unlike Nginx, where every site can be its
// own file, HAProxy can't have two frontends bind the same port — sites
// sharing a port share one frontend and are told apart by Host header, so
// adding a site edits that frontend in place.
type SiteSpec struct {
	// Names are the Host header values this site answers. None makes it
	// the frontend's default_backend: every request no host rule matched.
	Names      []string `json:"names"`
	Kind       string   `json:"kind"`       // "proxy", "loadbalance" or "redirect"
	ListenPort int      `json:"listenPort"` // plain-HTTP port inside the container, default 80

	Upstream        string   `json:"upstream"`        // proxy: host:port (an "http://" prefix is accepted)
	Backends        []string `json:"backends"`        // loadbalance: "host:port [weight=N] [backup]"
	Balance         string   `json:"balance"`         // loadbalance: "roundrobin" (default), "leastconn" or "source"
	HealthCheckPath string   `json:"healthCheckPath"` // optional HTTP check path; servers are TCP-checked either way

	RedirectTo   string `json:"redirectTo"`   // redirect: e.g. https://example.com; the path and query are kept
	RedirectCode int    `json:"redirectCode"` // 301 (default), 302, 307 or 308

	// CertPath is a PEM bundle (certificate and key together, HAProxy's
	// format) inside the container. Setting it serves the site over HTTPS
	// on 443 too.
	CertPath   string `json:"certPath"`
	ForceHTTPS bool   `json:"forceHttps"` // send plain HTTP to HTTPS; needs CertPath

	Websocket bool   `json:"websocket"` // keep upgraded connections open ("timeout tunnel")
	Timeout   string `json:"timeout"`   // backend "timeout server", e.g. 60s

	// Security headers, each optional and off by default.
	HSTS                  bool   `json:"hsts"` // only sent over HTTPS; needs CertPath
	XFrameOptions         string `json:"xFrameOptions"`
	NoSniff               bool   `json:"noSniff"`
	XSSProtection         bool   `json:"xssProtection"`
	ReferrerPolicy        string `json:"referrerPolicy"`
	ContentSecurityPolicy string `json:"contentSecurityPolicy"`
	CSPReportOnly         bool   `json:"cspReportOnly"`
	PermissionsPolicy     string `json:"permissionsPolicy"`
}

var (
	hostNameRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$`)
	hostPortRe  = regexp.MustCompile(`^[A-Za-z0-9._-]+:[0-9]{1,5}$`)
	lbBackendRe = regexp.MustCompile(`^([A-Za-z0-9._-]+:[0-9]{1,5})((?: (?:weight=[0-9]+|backup))*)$`)
	redirectRe  = regexp.MustCompile(`^https?://[^\s"'\\]+$`)
	urlPathRe   = regexp.MustCompile(`^/[^\s"'\\]*$`)
	filePathRe  = regexp.MustCompile(`^/[^\s"'\\]+$`)
	durationRe  = regexp.MustCompile(`^[0-9]+(ms|s|m|h|d)?$`)
	// Anything but a quote, backslash or newline: those could end the quoted
	// value early or start a new directive.
	headerValueRe = regexp.MustCompile(`^[^"\\\r\n]*$`)
	slugRe        = regexp.MustCompile(`[^a-z0-9]+`)
)

var referrerPolicies = map[string]bool{
	"": true, "no-referrer": true, "no-referrer-when-downgrade": true, "origin": true,
	"origin-when-cross-origin": true, "same-origin": true, "strict-origin": true,
	"strict-origin-when-cross-origin": true, "unsafe-url": true,
}

// Validate checks a spec and fills its defaults.
func (s *SiteSpec) Validate() error {
	var names []string
	seen := map[string]bool{}
	for _, n := range s.Names {
		for _, f := range strings.FieldsFunc(n, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
			f = strings.ToLower(f)
			if !hostNameRe.MatchString(f) {
				return fmt.Errorf("%q is not a valid domain name", f)
			}
			if !seen[f] {
				seen[f] = true
				names = append(names, f)
			}
		}
	}
	s.Names = names

	if s.ListenPort == 0 {
		s.ListenPort = 80
	}
	if s.ListenPort < 1 || s.ListenPort > 65535 {
		return fmt.Errorf("%d is not a valid port", s.ListenPort)
	}

	switch s.Kind {
	case "proxy":
		u := strings.TrimSpace(s.Upstream)
		if strings.HasPrefix(u, "https://") {
			return fmt.Errorf("HTTPS upstreams aren't supported by this form; add the site with the plain address, then edit its server line")
		}
		u = strings.TrimSuffix(strings.TrimPrefix(u, "http://"), "/")
		if !hostPortRe.MatchString(u) {
			return fmt.Errorf("proxy to must be host:port, e.g. app:3000")
		}
		s.Upstream = u
	case "loadbalance":
		var backends []string
		for _, b := range s.Backends {
			b = strings.Join(strings.Fields(b), " ")
			if b == "" {
				continue
			}
			if !lbBackendRe.MatchString(b) {
				return fmt.Errorf("%q is not host:port, optionally followed by weight=N or backup", b)
			}
			backends = append(backends, b)
		}
		if len(backends) == 0 {
			return fmt.Errorf("enter at least one backend")
		}
		s.Backends = backends
		switch s.Balance {
		case "":
			s.Balance = "roundrobin"
		case "roundrobin", "leastconn", "source":
		default:
			return fmt.Errorf("unknown balancing method %q", s.Balance)
		}
	case "redirect":
		to := strings.TrimSuffix(strings.TrimSpace(s.RedirectTo), "/")
		if !redirectRe.MatchString(to) {
			return fmt.Errorf("redirect to must be an http:// or https:// address")
		}
		s.RedirectTo = to
		if s.RedirectCode == 0 {
			s.RedirectCode = 301
		}
		switch s.RedirectCode {
		case 301, 302, 307, 308:
		default:
			return fmt.Errorf("redirect code must be 301, 302, 307 or 308")
		}
	default:
		return fmt.Errorf("unknown site type %q", s.Kind)
	}

	if s.Kind == "redirect" {
		s.HealthCheckPath = ""
	} else if s.HealthCheckPath != "" && !urlPathRe.MatchString(s.HealthCheckPath) {
		return fmt.Errorf("the health check path must start with / and contain no spaces or quotes")
	}

	if s.CertPath != "" {
		if !filePathRe.MatchString(s.CertPath) {
			return fmt.Errorf("the certificate must be an absolute path inside the container, with no spaces or quotes")
		}
		if s.ListenPort == 443 {
			return fmt.Errorf("with a certificate, HTTPS is served on 443; set the listen port to the plain-HTTP one, e.g. 80")
		}
	} else {
		s.ForceHTTPS, s.HSTS = false, false
	}

	if s.Timeout != "" && !durationRe.MatchString(s.Timeout) {
		return fmt.Errorf("the timeout must be a duration like 60s or 5m")
	}
	switch s.XFrameOptions {
	case "", "SAMEORIGIN", "DENY":
	default:
		return fmt.Errorf("X-Frame-Options must be SAMEORIGIN or DENY")
	}
	if !referrerPolicies[s.ReferrerPolicy] {
		return fmt.Errorf("%q is not a Referrer-Policy value", s.ReferrerPolicy)
	}
	for _, v := range []string{s.ContentSecurityPolicy, s.PermissionsPolicy} {
		if !headerValueRe.MatchString(v) {
			return fmt.Errorf("header values can't contain quotes, backslashes or line breaks")
		}
	}
	return nil
}

// siteSlug names a site's backend and host ACL after its first domain.
func siteSlug(names []string, port int) string {
	if len(names) == 0 {
		return fmt.Sprintf("default_%d", port)
	}
	return strings.Trim(slugRe.ReplaceAllString(names[0], "_"), "_")
}

// MainFile is the config file new sections go in when no existing one is
// the right place: the first -f path the container runs with, if it's a
// file, else the first file read.
func MainFile(files []dockerx.File, configPaths []string) string {
	for _, p := range configPaths {
		for _, f := range files {
			if f.Path == p {
				return p
			}
		}
	}
	if len(files) > 0 {
		return files[0].Path
	}
	return ""
}

// SiteChange is the new content of one file a site touches.
type SiteChange struct {
	Path    string
	Before  string
	After   string
	BaseSHA string
}

// quoteValue wraps a header value for http-response set-header. "%" starts
// a log-format expression there, so a literal one is doubled — verified
// against a real container that "100%%" is sent as "100%".
func quoteValue(v string) string {
	return `"` + strings.ReplaceAll(v, "%", "%%") + `"`
}

func (s SiteSpec) headerLines() []string {
	set := func(name, value string) string {
		return fmt.Sprintf("    http-response set-header %s %s", name, quoteValue(value))
	}
	var out []string
	if s.HSTS {
		// Browsers ignore HSTS over plain HTTP; sending it only over TLS keeps
		// the plain-HTTP responses honest about that.
		out = append(out, set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")+" if { ssl_fc }")
	}
	if s.XFrameOptions != "" {
		out = append(out, set("X-Frame-Options", s.XFrameOptions))
	}
	if s.NoSniff {
		out = append(out, set("X-Content-Type-Options", "nosniff"))
	}
	if s.XSSProtection {
		out = append(out, set("X-XSS-Protection", "1; mode=block"))
	}
	if s.ReferrerPolicy != "" {
		out = append(out, set("Referrer-Policy", s.ReferrerPolicy))
	}
	if s.ContentSecurityPolicy != "" {
		name := "Content-Security-Policy"
		if s.CSPReportOnly {
			name += "-Report-Only"
		}
		out = append(out, set(name, s.ContentSecurityPolicy))
	}
	if s.PermissionsPolicy != "" {
		out = append(out, set("Permissions-Policy", s.PermissionsPolicy))
	}
	return out
}

// backendBlock is the site's own backend. Everything that's per-site —
// forcing HTTPS, redirecting, headers — lives here rather than on the
// shared frontend, so adding or removing a site never changes how the
// frontend treats any other site. Servers always get "check", so the
// Backends tab can show whether each one is up.
func (s SiteSpec) backendBlock(name string) []string {
	b := []string{"backend " + name, "    mode http"}
	if s.Kind == "loadbalance" {
		b = append(b, "    balance "+s.Balance)
	}
	if s.HealthCheckPath != "" {
		b = append(b, "    option httpchk GET "+s.HealthCheckPath)
	}
	if s.Timeout != "" {
		b = append(b, "    timeout server "+s.Timeout)
	}
	if s.Websocket {
		b = append(b, "    timeout tunnel 1h")
	}
	if s.ForceHTTPS {
		b = append(b, "    http-request redirect scheme https code 301 unless { ssl_fc }")
	}
	if s.Kind == "redirect" {
		b = append(b, fmt.Sprintf("    http-request redirect prefix %s code %d", s.RedirectTo, s.RedirectCode))
	}
	b = append(b, s.headerLines()...)
	switch s.Kind {
	case "proxy":
		b = append(b, "    server srv1 "+s.Upstream+" check")
	case "loadbalance":
		for i, be := range s.Backends {
			m := lbBackendRe.FindStringSubmatch(be)
			line := fmt.Sprintf("    server srv%d %s check", i+1, m[1])
			for _, opt := range strings.Fields(m[2]) {
				if opt == "backup" {
					line += " backup"
				} else {
					line += " weight " + strings.TrimPrefix(opt, "weight=")
				}
			}
			b = append(b, line)
		}
	}
	return b
}

// Render adds the site to the config made of files (in the order HAProxy
// loads them) and returns each file it changes, plus a one-line summary.
// mainPath is where a new frontend goes when no existing one serves the
// port. Nothing is written: Apply re-checks every file and dry-runs the
// result with haproxy -c first.
func (s SiteSpec) Render(files []dockerx.File, mainPath string) ([]SiteChange, string, error) {
	sections := parseFiles(files)
	modes := effectiveModes(sections)
	taken := map[string]bool{}
	for _, sec := range sections {
		if sec.name != "" {
			taken[sec.name] = true
		}
	}

	slug := siteSlug(s.Names, s.ListenPort)
	backend := "site_" + slug
	if taken[backend] {
		return nil, "", fmt.Errorf("a backend named %s already exists; edit its file instead, or use other domain names", backend)
	}
	var route []string // nil means "make it the default_backend"
	if len(s.Names) > 0 {
		acl := "host_" + slug
		route = []string{
			fmt.Sprintf("    acl %s req.hdr(host),field(1,:) -i %s", acl, strings.Join(s.Names, " ")),
			fmt.Sprintf("    use_backend %s if %s", backend, acl),
		}
	}

	httpIdx, err := findFrontend(sections, modes, s.ListenPort, false)
	if err != nil {
		return nil, "", err
	}
	httpsIdx := -1
	if s.CertPath != "" {
		if httpsIdx, err = findFrontend(sections, modes, 443, true); err != nil {
			return nil, "", err
		}
	}

	byPath := map[string]dockerx.File{}
	for _, f := range files {
		byPath[f.Path] = f
	}
	home := mainPath
	if httpIdx >= 0 {
		home = sections[httpIdx].file
	}
	if _, ok := byPath[home]; !ok {
		return nil, "", fmt.Errorf("the config file %s wasn't read; inspect again and retry", home)
	}

	// Edits to existing sections, applied per file bottom-up so each header
	// line number still points at its section when its turn comes.
	type edit struct {
		header int
		apply  func([]string) []string
	}
	edits := map[string][]edit{}
	var replaced []string
	addRouteTo := func(idx int, cert string) {
		sec := sections[idx]
		edits[sec.file] = append(edits[sec.file], edit{sec.line - 1, func(lines []string) []string {
			if cert != "" {
				addCert(lines, sec.line-1, cert)
			}
			lines, old := addRoute(lines, sec.line-1, route, backend)
			if old != "" {
				replaced = append(replaced, fmt.Sprintf("default_backend %s in %s", old, sec.describe()))
			}
			return lines
		}})
	}
	var newBlocks [][]string
	unique := func(base string) string {
		name := base
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s_%d", base, n)
		}
		taken[name] = true
		return name
	}

	via := ""
	if httpIdx >= 0 {
		via = sections[httpIdx].describe()
		addRouteTo(httpIdx, "")
	} else {
		name := unique(fmt.Sprintf("http_%d", s.ListenPort))
		via = "new frontend " + name
		newBlocks = append(newBlocks, frontendBlock(name, fmt.Sprintf("*:%d", s.ListenPort), route, backend))
	}
	if s.CertPath != "" {
		switch {
		case httpsIdx >= 0 && httpsIdx == httpIdx:
			// One section serves both ports: route once, just add the cert.
			sec := sections[httpsIdx]
			edits[sec.file] = append(edits[sec.file], edit{sec.line - 1, func(lines []string) []string {
				addCert(lines, sec.line-1, s.CertPath)
				return lines
			}})
		case httpsIdx >= 0:
			via += " and " + sections[httpsIdx].describe()
			addRouteTo(httpsIdx, s.CertPath)
		default:
			name := unique("https_443")
			via += " and new frontend " + name
			newBlocks = append(newBlocks, frontendBlock(name, "*:443 ssl crt "+s.CertPath, route, backend))
		}
	}
	newBlocks = append(newBlocks, s.backendBlock(backend))

	var changes []SiteChange
	paths := make([]string, 0, len(edits)+1)
	for p := range edits {
		paths = append(paths, p)
	}
	if _, ok := edits[home]; !ok {
		paths = append(paths, home)
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := byPath[p]
		lines := strings.Split(f.Content, "\n")
		es := edits[p]
		sort.Slice(es, func(a, b int) bool { return es[a].header > es[b].header })
		for _, e := range es {
			lines = e.apply(lines)
		}
		if p == home {
			lines = appendBlocks(lines, newBlocks...)
		}
		changes = append(changes, SiteChange{Path: p, Before: f.Content, After: strings.Join(lines, "\n"), BaseSHA: f.SHA})
	}

	kind := map[string]string{"proxy": "reverse proxy", "loadbalance": "load balancer", "redirect": "redirect"}[s.Kind]
	hosts := strings.Join(s.Names, ", ")
	if hosts == "" {
		hosts = fmt.Sprintf("every other request on :%d", s.ListenPort)
	}
	target := map[string]string{
		"proxy":       "→ " + s.Upstream,
		"loadbalance": fmt.Sprintf("→ %d servers (%s)", len(s.Backends), s.Balance),
		"redirect":    "→ " + s.RedirectTo,
	}[s.Kind]
	summary := fmt.Sprintf("Add %s for %s %s via %s, backend %s", kind, hosts, target, via, backend)
	if len(replaced) > 0 {
		summary += "; replaces " + strings.Join(replaced, " and ")
	}
	return changes, summary, nil
}

// findFrontend returns the frontend/listen section binding port, -1 if
// none, or an error if the section serving it can't take a host rule.
func findFrontend(sections []section, modes []string, port int, tls bool) (int, error) {
	for i, sec := range sections {
		if sec.kind != "frontend" && sec.kind != "listen" {
			continue
		}
		for _, b := range sec.binds {
			if p, ok := bindPort(b); !ok || p != port {
				continue
			}
			isTLS := false
			for _, t := range sec.tlsBinds {
				if t == b {
					isTLS = true
				}
			}
			switch {
			case isTLS && !tls:
				return -1, fmt.Errorf("port %d is served with TLS by %s; set the listen port to the plain-HTTP one", port, sec.describe())
			case !isTLS && tls:
				return -1, fmt.Errorf("port %d is served without TLS by %s, so HTTPS can't be added there", port, sec.describe())
			case modes[i] != "http":
				return -1, fmt.Errorf("port %d is served by %s in TCP mode, which can't tell sites apart by domain", port, sec.describe())
			case sec.statsEnabled:
				return -1, fmt.Errorf("port %d is the stats page (%s); pick another port", port, sec.describe())
			}
			return i, nil
		}
	}
	return -1, nil
}

func frontendBlock(name, bind string, route []string, backend string) []string {
	b := []string{"frontend " + name, "    bind " + bind, "    mode http"}
	if route == nil {
		return append(b, "    default_backend "+backend)
	}
	return append(b, route...)
}

// sectionEnd is the index just past the section whose header is at h.
func sectionEnd(lines []string, h int) int {
	j := h + 1
	for j < len(lines) && !isSectionHeader(lines[j]) {
		j++
	}
	return j
}

// lastContent is the last line in (h, end) that isn't blank or a comment,
// or h when there's none — so trailing comments that introduce the next
// section stay with it.
func lastContent(lines []string, h, end int) int {
	for j := end - 1; j > h; j-- {
		if t := strings.TrimSpace(lines[j]); t != "" && !strings.HasPrefix(t, "#") {
			return j
		}
	}
	return h
}

func insertAt(lines []string, at int, add []string) []string {
	out := make([]string, 0, len(lines)+len(add))
	out = append(out, lines[:at]...)
	out = append(out, add...)
	return append(out, lines[at:]...)
}

// addRoute adds the site to the section with header h, or with no route,
// makes it the section's default_backend — returning the default it
// replaced, if any. A host rule goes right after the last site the panel
// added (so sites stay together, acl beside its use_backend, in the order
// they were added), else before the section's first use_backend or
// default_backend: a domain-specific site is checked before any
// hand-written catch-all rule, which would otherwise swallow its traffic.
func addRoute(lines []string, h int, route []string, backend string) ([]string, string) {
	end := sectionEnd(lines, h)
	if route == nil {
		for j := h + 1; j < end; j++ {
			f := strings.Fields(lines[j])
			if len(f) > 1 && f[0] == "default_backend" {
				indent := lines[j][:len(lines[j])-len(strings.TrimLeft(lines[j], " \t"))]
				lines[j] = indent + "default_backend " + backend
				return lines, f[1]
			}
		}
		return insertAt(lines, lastContent(lines, h, end)+1, []string{"    default_backend " + backend}), ""
	}
	lastSite, firstRule := -1, -1
	for j := h + 1; j < end; j++ {
		f := strings.Fields(lines[j])
		if len(f) == 0 || (f[0] != "use_backend" && f[0] != "default_backend") {
			continue
		}
		if firstRule < 0 {
			firstRule = j
		}
		if len(f) == 4 && f[0] == "use_backend" && strings.HasPrefix(f[1], "site_") && f[2] == "if" && strings.HasPrefix(f[3], "host_") {
			lastSite = j
		}
	}
	switch {
	case lastSite >= 0:
		return insertAt(lines, lastSite+1, route), ""
	case firstRule >= 0:
		return insertAt(lines, firstRule, route), ""
	}
	return insertAt(lines, lastContent(lines, h, end)+1, route), ""
}

// addCert adds a certificate to the first TLS bind in the section with
// header h, unless it's already there. A bind can carry several "crt"s;
// HAProxy picks one per connection by SNI.
func addCert(lines []string, h int, cert string) {
	end := sectionEnd(lines, h)
	for j := h + 1; j < end; j++ {
		f := strings.Fields(lines[j])
		if len(f) < 2 || f[0] != "bind" {
			continue
		}
		hasTLS := false
		for k, t := range f {
			if t == "ssl" {
				hasTLS = true
			}
			if t == "crt" && k+1 < len(f) && f[k+1] == cert {
				return
			}
		}
		if hasTLS {
			lines[j] = strings.TrimRight(lines[j], " \t") + " crt " + cert
			return
		}
	}
}

// appendBlocks adds sections at the end of a file, each after a blank
// line, ending with a newline — HAProxy rejects a last line without one.
func appendBlocks(lines []string, blocks ...[]string) []string {
	if len(blocks) == 0 {
		return lines
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	for _, b := range blocks {
		lines = append(lines, "")
		lines = append(lines, b...)
	}
	return append(lines, "")
}
