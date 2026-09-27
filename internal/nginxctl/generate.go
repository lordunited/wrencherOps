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
}

var (
	serverNameRe = regexp.MustCompile(`^(_|[A-Za-z0-9*][A-Za-z0-9*._-]*|~.+)$`)
	urlRe        = regexp.MustCompile(`^https?://[^\s;{}'"]+$`)
	backendRe    = regexp.MustCompile(`^[A-Za-z0-9._-]+:[0-9]{1,5}( (weight=[0-9]+|max_fails=[0-9]+|fail_timeout=[0-9]+[smh]?|backup|down))*$`)
	sizeRe       = regexp.MustCompile(`^[0-9]+[kKmMgG]?$`)
	timeRe       = regexp.MustCompile(`^[0-9]+(ms|s|m|h)?$`)
	pathRe       = regexp.MustCompile(`^/[^\s;{}'"]+$`)
	fileNameRe   = regexp.MustCompile(`[^a-z0-9._-]+`)
)

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
	return nil
}

// Render writes the server block (and, for load balancing, its upstream)
// as the content of one config file.
func (s ServerSpec) Render() string {
	var b strings.Builder
	names := strings.Join(s.Names, " ")
	https := s.CertPath != ""
	fmt.Fprintf(&b, "# Managed with the DevOps Panel. Safe to edit by hand.\n")

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
		body("    ")
		b.WriteString("}\n")
	}
	return b.String()
}
