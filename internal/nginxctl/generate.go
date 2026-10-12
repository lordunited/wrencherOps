// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package nginxctl

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

func sortOverview(ov *Overview) {
	sort.SliceStable(ov.Servers, func(a, b int) bool {
		if ov.Servers[a].File != ov.Servers[b].File {
			return ov.Servers[a].File < ov.Servers[b].File
		}
		return ov.Servers[a].Line < ov.Servers[b].Line
	})
	sort.SliceStable(ov.Upstreams, func(a, b int) bool { return ov.Upstreams[a].Name < ov.Upstreams[b].Name })
	sort.Strings(ov.ParseErrors)
}

// ServerSpec describes a new server block the panel writes as its own file.
type ServerSpec struct {
	Names      []string `json:"names"`      // server_name values
	Kind       string   `json:"kind"`       // "proxy", "loadbalance" or "redirect"
	ListenPort int      `json:"listenPort"` // plain HTTP port inside the container, default 80

	Upstream string   `json:"upstream"` // proxy: e.g. http://app:3000
	Backends []string `json:"backends"` // loadbalance: host:port, one per backend
	Method   string   `json:"method"`   // loadbalance: "", "least_conn" or "ip_hash"

	RedirectTo   string `json:"redirectTo"`   // redirect: e.g. https://example.com
	RedirectCode int    `json:"redirectCode"` // 301 (default) or 302

	CertPath   string `json:"certPath"` // enables HTTPS on 443 when set, with KeyPath
	KeyPath    string `json:"keyPath"`
	ForceHTTPS bool   `json:"forceHttps"` // with a certificate, send plain HTTP to HTTPS

	Websocket   bool   `json:"websocket"`   // pass Upgrade headers through
	MaxBodySize string `json:"maxBodySize"` // client_max_body_size, e.g. 50m
	Timeout     string `json:"timeout"`     // proxy read/send timeout, e.g. 60s

	// Security headers, each optional and off by default.
	HSTS                  bool   `json:"hsts"`                  // Strict-Transport-Security; only sent on the HTTPS server block
	XFrameOptions         string `json:"xFrameOptions"`         // "", "SAMEORIGIN" or "DENY"
	NoSniff               bool   `json:"noSniff"`               // X-Content-Type-Options: nosniff
	XSSProtection         bool   `json:"xssProtection"`         // X-XSS-Protection: legacy, IE11/old-browser only
	ReferrerPolicy        string `json:"referrerPolicy"`        // "" or a valid Referrer-Policy value
	ContentSecurityPolicy string `json:"contentSecurityPolicy"` // free text, e.g. "default-src 'self'"
	CSPReportOnly         bool   `json:"cspReportOnly"`         // send as Content-Security-Policy-Report-Only instead — logs violations without blocking anything, for testing a policy before enforcing it
	PermissionsPolicy     string `json:"permissionsPolicy"`     // free text, e.g. "geolocation=(), camera=()"

	// CORS. When enabled, OPTIONS preflight requests get a 204 straight
	// from Nginx (proxy/loadbalance only) instead of reaching the backend.
	CORS        bool   `json:"cors"`
	CORSOrigin  string `json:"corsOrigin"`  // "*", one origin, or several — one per line or comma-separated. With more than one, Nginx can only ever answer with ONE, so this generates a "map" that reflects $http_origin back only when it's in the list (and denies otherwise) — the safe alternative to blindly reflecting any Origin, which defeats CORS entirely.
	CORSMethods string `json:"corsMethods"` // default "GET, POST, PUT, PATCH, DELETE, OPTIONS"
	CORSHeaders string `json:"corsHeaders"` // default "Authorization, Content-Type"
	// CORSCredentials sets Access-Control-Allow-Credentials. Needs a
	// specific origin (one, or several via the map above) — never "*".
	CORSCredentials bool `json:"corsCredentials"`
}

// corsOrigins splits CORSOrigin into individual origins: one per line, or
// comma-separated within a line, trimmed and with blanks dropped.
func (s ServerSpec) corsOrigins() []string {
	var out []string
	for _, line := range strings.Split(s.CORSOrigin, "\n") {
		for _, part := range strings.Split(line, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

// corsMapVarName derives an Nginx variable name for this server's CORS
// origin map from the same slug used for its filename, so several
// generated files can each define their own map without colliding —
// "map" variables live in Nginx's single global namespace, not scoped to
// the file that defines them.
func corsMapVarName(names []string) string {
	slug := strings.NewReplacer(".", "_", "-", "_").Replace(strings.TrimSuffix(FileName(names), ".conf"))
	return "cors_origin_" + slug
}

var (
	serverNameRe = regexp.MustCompile(`^(_|[A-Za-z0-9*][A-Za-z0-9*._-]*|~.+)$`)
	urlRe        = regexp.MustCompile(`^https?://[^\s;{}'"]+$`)
	backendRe    = regexp.MustCompile(`^[A-Za-z0-9._-]+:[0-9]{1,5}( (weight=[0-9]+|max_fails=[0-9]+|fail_timeout=[0-9]+[smh]?|backup|down))*$`)
	sizeRe       = regexp.MustCompile(`^[0-9]+[kKmMgG]?$`)
	timeRe       = regexp.MustCompile(`^[0-9]+(ms|s|m|h)?$`)
	pathRe       = regexp.MustCompile(`^/[^\s;{}'"]+$`)
	fileNameRe   = regexp.MustCompile(`[^a-z0-9._-]+`)
	headerValRe  = regexp.MustCompile(`^[^"\r\n]*$`) // anything but a quote or a newline, so it can't break out of add_header's quoted string or inject a new directive
	corsOriginRe = regexp.MustCompile(`^(\*|https?://[^\s;{}'"]+)$`)
	corsListRe   = regexp.MustCompile(`^[A-Za-z0-9, -]*$`)
)

var referrerPolicies = map[string]bool{
	"": true, "no-referrer": true, "no-referrer-when-downgrade": true, "origin": true,
	"origin-when-cross-origin": true, "same-origin": true, "strict-origin": true,
	"strict-origin-when-cross-origin": true, "unsafe-url": true,
}

// FileName is the file name the panel uses for a server block with these
// names: the first name, lowercased and made filesystem-safe.
func FileName(names []string) string {
	base := "server"
	if len(names) > 0 && names[0] != "_" {
		base = strings.Trim(fileNameRe.ReplaceAllString(strings.ToLower(strings.TrimPrefix(names[0], "*.")), "-"), "-.")
	}
	if base == "" {
		base = "server"
	}
	return base + ".conf"
}

func upstreamName(names []string) string {
	return strings.ReplaceAll(strings.TrimSuffix(FileName(names), ".conf"), ".", "_") + "_backend"
}

// Validate checks a spec and fills defaults.
func (s *ServerSpec) Validate() error {
	var names []string
	for _, n := range s.Names {
		for _, f := range strings.Fields(n) {
			if !serverNameRe.MatchString(f) {
				return fmt.Errorf("%q is not a valid server name", f)
			}
			names = append(names, f)
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("enter at least one domain, or _ to match any name")
	}
	s.Names = names
	if s.ListenPort == 0 {
		s.ListenPort = 80
	}
	if s.ListenPort < 1 || s.ListenPort > 65535 {
		return fmt.Errorf("listen port must be between 1 and 65535")
	}
	switch s.Kind {
	case "proxy":
		s.Upstream = strings.TrimSpace(s.Upstream)
		if !urlRe.MatchString(s.Upstream) {
			return fmt.Errorf("enter the address to proxy to as a URL, e.g. http://app:3000")
		}
	case "loadbalance":
		var bs []string
		for _, b := range s.Backends {
			b = strings.Join(strings.Fields(b), " ")
			if b == "" {
				continue
			}
			if !backendRe.MatchString(b) {
				return fmt.Errorf("%q is not a backend like 10.0.0.5:8080 (optionally followed by weight=3, backup, ...)", b)
			}
			bs = append(bs, b)
		}
		if len(bs) == 0 {
			return fmt.Errorf("add at least one backend")
		}
		s.Backends = bs
		if s.Method != "" && s.Method != "least_conn" && s.Method != "ip_hash" {
			return fmt.Errorf("unknown balancing method %q", s.Method)
		}
	case "redirect":
		s.RedirectTo = strings.TrimSpace(s.RedirectTo)
		if !urlRe.MatchString(s.RedirectTo) {
			return fmt.Errorf("enter where to redirect as a URL, e.g. https://example.com")
		}
		if s.RedirectCode == 0 {
			s.RedirectCode = 301
		}
		if s.RedirectCode != 301 && s.RedirectCode != 302 && s.RedirectCode != 307 && s.RedirectCode != 308 {
			return fmt.Errorf("redirect code must be 301, 302, 307 or 308")
		}
	default:
		return fmt.Errorf("unknown server type %q", s.Kind)
	}
	if (s.CertPath == "") != (s.KeyPath == "") {
		return fmt.Errorf("HTTPS needs both a certificate and a key path")
	}
	for _, p := range []string{s.CertPath, s.KeyPath} {
		if p != "" && !pathRe.MatchString(p) {
			return fmt.Errorf("%q is not an absolute file path inside the container", p)
		}
	}
	if s.MaxBodySize != "" && !sizeRe.MatchString(s.MaxBodySize) {
		return fmt.Errorf("max body size %q should look like 10m or 1g", s.MaxBodySize)
	}
	if s.Timeout != "" && !timeRe.MatchString(s.Timeout) {
		return fmt.Errorf("timeout %q should look like 60s or 5m", s.Timeout)
	}
	if s.XFrameOptions != "" && s.XFrameOptions != "SAMEORIGIN" && s.XFrameOptions != "DENY" {
		return fmt.Errorf("X-Frame-Options must be SAMEORIGIN or DENY")
	}
	if !referrerPolicies[s.ReferrerPolicy] {
		return fmt.Errorf("%q is not a valid Referrer-Policy value", s.ReferrerPolicy)
	}
	if !headerValRe.MatchString(s.ContentSecurityPolicy) {
		return fmt.Errorf("Content-Security-Policy can't contain a quote or a line break")
	}
	if !headerValRe.MatchString(s.PermissionsPolicy) {
		return fmt.Errorf("Permissions-Policy can't contain a quote or a line break")
	}
	if s.CORS {
		origins := s.corsOrigins()
		switch len(origins) {
		case 0:
			// Defaults to "*" at render time — same wildcard/credentials
			// conflict as an explicit "*".
			if s.CORSCredentials {
				return fmt.Errorf("Access-Control-Allow-Credentials needs a specific CORS origin, not * — browsers reject that combination")
			}
		case 1:
			if !corsOriginRe.MatchString(origins[0]) {
				return fmt.Errorf("CORS origin must be * or a URL like https://app.example.com")
			}
			if s.CORSCredentials && origins[0] == "*" {
				return fmt.Errorf("Access-Control-Allow-Credentials needs a specific CORS origin, not * — browsers reject that combination")
			}
		default:
			for _, o := range origins {
				if !urlRe.MatchString(o) {
					return fmt.Errorf("%q is not a valid CORS origin — with more than one, each must be a URL like https://app.example.com (no *)", o)
				}
			}
		}
		if !corsListRe.MatchString(s.CORSMethods) {
			return fmt.Errorf("CORS methods can only contain letters, commas and spaces")
		}
		if !corsListRe.MatchString(s.CORSHeaders) {
			return fmt.Errorf("CORS headers can only contain letters, commas, hyphens and spaces")
		}
	}
	return nil
}

// Render writes the server block (and, for load balancing, its upstream)
// as the content of one config file.
func (s ServerSpec) Render() string {
	var b strings.Builder
	names := strings.Join(s.Names, " ")
	https := s.CertPath != ""
	fmt.Fprintf(&b, "# Managed with the DevOps Panel. Safe to edit by hand.\n")

	// With one CORS origin (or none, or "*"), the literal value goes
	// straight into Access-Control-Allow-Origin. With several, Nginx can
	// only ever answer with ONE, so a "map" reflects $http_origin back
	// only when it's actually in the list — anything else gets "", which
	// browsers treat as CORS access denied.
	corsOriginExpr := "*"
	corsOriginVaries := false
	if origins := s.corsOrigins(); s.CORS {
		switch len(origins) {
		case 0:
			corsOriginExpr = "*"
		case 1:
			corsOriginExpr = origins[0]
		default:
			varName := corsMapVarName(s.Names)
			fmt.Fprintf(&b, "\nmap $http_origin $%s {\n", varName)
			b.WriteString("    default \"\";\n")
			for _, o := range origins {
				fmt.Fprintf(&b, "    %s $http_origin;\n", o)
			}
			b.WriteString("}\n")
			corsOriginExpr = "$" + varName
			corsOriginVaries = true
		}
	}

	target := s.Upstream
	if s.Kind == "loadbalance" {
		up := upstreamName(s.Names)
		fmt.Fprintf(&b, "\nupstream %s {\n", up)
		if s.Method != "" {
			fmt.Fprintf(&b, "    %s;\n", s.Method)
		}
		for _, be := range s.Backends {
			fmt.Fprintf(&b, "    server %s;\n", be)
		}
		b.WriteString("    keepalive 16;\n}\n")
		target = "http://" + up
	}

	// corsHeaders writes the Access-Control-* lines on their own, so
	// proxy/loadbalance can place them directly inside "location /" —
	// verified against a real nginx: add_header declared in the *same*
	// block as a nested "if" does reach a "return" inside it, but
	// add_header merely inherited from the parent server/location does
	// not, so these must live in the same location as the OPTIONS check
	// below rather than up at server level.
	corsHeaders := func(indent string) {
		if !s.CORS {
			return
		}
		methods, allowHeaders := s.CORSMethods, s.CORSHeaders
		if methods == "" {
			methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		}
		if allowHeaders == "" {
			allowHeaders = "Authorization, Content-Type"
		}
		if corsOriginVaries {
			// The response now depends on the request's Origin — without
			// this, a cache or CDN could serve one client's allowed
			// origin to a different one entirely.
			fmt.Fprintf(&b, "%sadd_header Vary Origin always;\n", indent)
		}
		fmt.Fprintf(&b, "%sadd_header Access-Control-Allow-Origin \"%s\" always;\n", indent, corsOriginExpr)
		fmt.Fprintf(&b, "%sadd_header Access-Control-Allow-Methods \"%s\" always;\n", indent, methods)
		fmt.Fprintf(&b, "%sadd_header Access-Control-Allow-Headers \"%s\" always;\n", indent, allowHeaders)
		if s.CORSCredentials {
			fmt.Fprintf(&b, "%sadd_header Access-Control-Allow-Credentials \"true\" always;\n", indent)
		}
	}

	// headers writes every configured non-CORS security header as
	// add_header at server level, so a location that sets none of its own
	// inherits them on every real response. HSTS only goes on the block a
	// browser reaches over TLS. CORS headers are handled by body() instead:
	// for redirect there's no location to put them in, so they still go
	// here; for proxy/loadbalance they belong inside "location /" (see
	// corsHeaders above), so this skips them there to avoid emitting twice.
	headers := func(indent string, https bool) {
		if https && s.HSTS {
			fmt.Fprintf(&b, "%sadd_header Strict-Transport-Security \"max-age=63072000; includeSubDomains\" always;\n", indent)
		}
		if s.XFrameOptions != "" {
			fmt.Fprintf(&b, "%sadd_header X-Frame-Options %s always;\n", indent, s.XFrameOptions)
		}
		if s.NoSniff {
			fmt.Fprintf(&b, "%sadd_header X-Content-Type-Options nosniff always;\n", indent)
		}
		if s.XSSProtection {
			fmt.Fprintf(&b, "%sadd_header X-XSS-Protection \"1; mode=block\" always;\n", indent)
		}
		if s.ReferrerPolicy != "" {
			fmt.Fprintf(&b, "%sadd_header Referrer-Policy %s always;\n", indent, s.ReferrerPolicy)
		}
		if s.ContentSecurityPolicy != "" {
			name := "Content-Security-Policy"
			if s.CSPReportOnly {
				name = "Content-Security-Policy-Report-Only"
			}
			fmt.Fprintf(&b, "%sadd_header %s \"%s\" always;\n", indent, name, s.ContentSecurityPolicy)
		}
		if s.PermissionsPolicy != "" {
			fmt.Fprintf(&b, "%sadd_header Permissions-Policy \"%s\" always;\n", indent, s.PermissionsPolicy)
		}
		if s.Kind == "redirect" {
			corsHeaders(indent)
		}
	}

	body := func(indent string) {
		if s.MaxBodySize != "" {
			fmt.Fprintf(&b, "%sclient_max_body_size %s;\n\n", indent, s.MaxBodySize)
		}
		if s.Kind == "redirect" {
			fmt.Fprintf(&b, "%sreturn %d %s$request_uri;\n", indent, s.RedirectCode, strings.TrimRight(s.RedirectTo, "/"))
			return
		}
		fmt.Fprintf(&b, "%slocation / {\n", indent)
		in := indent + "    "
		corsHeaders(in)
		if s.CORS {
			// Answer the preflight straight from Nginx — the backend never
			// sees it, and it picks up the Access-Control-* headers just
			// above since they're declared in this same location. No
			// add_header goes inside this "if": even one (Content-Length,
			// say) would reset inheritance for its own context and wipe
			// out every header the location just set — verified against a
			// real container, where that's exactly what happened.
			fmt.Fprintf(&b, "%sif ($request_method = OPTIONS) {\n", in)
			fmt.Fprintf(&b, "%s    return 204;\n", in)
			fmt.Fprintf(&b, "%s}\n", in)
		}
		fmt.Fprintf(&b, "%sproxy_pass %s;\n", in, target)
		fmt.Fprintf(&b, "%sproxy_http_version 1.1;\n", in)
		fmt.Fprintf(&b, "%sproxy_set_header Host $host;\n", in)
		fmt.Fprintf(&b, "%sproxy_set_header X-Real-IP $remote_addr;\n", in)
		fmt.Fprintf(&b, "%sproxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;\n", in)
		fmt.Fprintf(&b, "%sproxy_set_header X-Forwarded-Proto $scheme;\n", in)
		if s.Websocket {
			fmt.Fprintf(&b, "%sproxy_set_header Upgrade $http_upgrade;\n", in)
			fmt.Fprintf(&b, "%sproxy_set_header Connection \"upgrade\";\n", in)
		} else if s.Kind == "loadbalance" {
			fmt.Fprintf(&b, "%sproxy_set_header Connection \"\";\n", in)
		}
		if s.Timeout != "" {
			fmt.Fprintf(&b, "%sproxy_read_timeout %s;\n", in, s.Timeout)
			fmt.Fprintf(&b, "%sproxy_send_timeout %s;\n", in, s.Timeout)
		}
		fmt.Fprintf(&b, "%s}\n", indent)
	}

	fmt.Fprintf(&b, "\nserver {\n")
	fmt.Fprintf(&b, "    listen %d;\n", s.ListenPort)
	fmt.Fprintf(&b, "    server_name %s;\n\n", names)
	if https && s.ForceHTTPS {
		b.WriteString("    return 301 https://$host$request_uri;\n")
	} else {
		// This block is plain HTTP even when a cert exists for the HTTPS
		// block below, so it never gets HSTS.
		headers("    ", false)
		body("    ")
	}
	b.WriteString("}\n")

	if https {
		fmt.Fprintf(&b, "\nserver {\n")
		// "listen ... http2" works on every Nginx version; newer ones only
		// note that "http2 on;" is the preferred spelling.
		b.WriteString("    listen 443 ssl http2;\n")
		fmt.Fprintf(&b, "    server_name %s;\n\n", names)
		fmt.Fprintf(&b, "    ssl_certificate %s;\n", s.CertPath)
		fmt.Fprintf(&b, "    ssl_certificate_key %s;\n", s.KeyPath)
		b.WriteString("    ssl_protocols TLSv1.2 TLSv1.3;\n")
		b.WriteString("    ssl_session_cache shared:SSL:10m;\n\n")
		headers("    ", true)
		body("    ")
		b.WriteString("}\n")
	}
	return b.String()
}
