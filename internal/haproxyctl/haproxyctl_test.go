// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package haproxyctl

import (
	"io"
	"strings"
	"testing"

	"devops-wrapper/internal/deploy"
)

// The panel's own seed config must keep monitor-uri as its health probe
// (adding a stats page must not change what Sync checks) while also
// exposing a stats page, or the Backends tab has nothing to read.
func TestPanelDefaultConfigKeepsMonitorURIAndEnablesStats(t *testing.T) {
	sections := parseSections(deploy.HAProxyDefaultConfig)
	if p := DetermineProbe(sections); p.Kind != "monitor-uri" || p.Port != 80 || p.Path != "/healthz" {
		t.Errorf("DetermineProbe(default) = %+v, want monitor-uri /healthz on :80", p)
	}
	port, uri, ok := statsEndpoint(sections)
	if !ok || port != deploy.HAProxyStatsPort || uri != "/haproxy?stats" {
		t.Errorf("statsEndpoint(default) = %d, %q, %v; want %d, /haproxy?stats, true", port, uri, ok, deploy.HAProxyStatsPort)
	}
}

func TestConfigPaths(t *testing.T) {
	if got := configPaths(nil); len(got) != 1 || got[0] != "/usr/local/etc/haproxy/haproxy.cfg" {
		t.Errorf("default configPaths = %v", got)
	}
	if got := configPaths([]string{"haproxy", "-f", "/etc/haproxy/haproxy.cfg"}); len(got) != 1 || got[0] != "/etc/haproxy/haproxy.cfg" {
		t.Errorf("single -f = %v", got)
	}
	got := configPaths([]string{"haproxy", "-f", "/etc/haproxy/haproxy.cfg", "-f", "/etc/haproxy/conf.d", "-db"})
	if len(got) != 2 || got[0] != "/etc/haproxy/haproxy.cfg" || got[1] != "/etc/haproxy/conf.d" {
		t.Errorf("repeated -f = %v", got)
	}
}

func TestBindPort(t *testing.T) {
	cases := map[string]int{
		"*:80":            80,
		":80":             80,
		"0.0.0.0:8404":    8404,
		"127.0.0.1:80-85": 80,
	}
	for bind, want := range cases {
		got, ok := bindPort(bind)
		if !ok || got != want {
			t.Errorf("bindPort(%q) = %d, %v; want %d", bind, got, ok, want)
		}
	}
	if _, ok := bindPort("/var/run/haproxy.sock"); ok {
		t.Errorf("a unix socket path must not match a port")
	}
}

func TestDetermineProbePrefersMonitorURI(t *testing.T) {
	cfg := `
global
    log stdout format raw local0

defaults
    mode http

listen stats
    bind *:8404
    stats enable

frontend main
    bind *:80
    monitor-uri /healthz
    default_backend app
`
	probe := DetermineProbe(parseSections(cfg))
	if probe.Kind != "monitor-uri" || probe.Port != 80 || probe.Path != "/healthz" || probe.Section != "frontend main" {
		t.Errorf("probe = %+v", probe)
	}
}

func TestDetermineProbeFallsBackToStats(t *testing.T) {
	cfg := `
defaults
    mode http

listen stats
    bind *:8404
    stats enable
    stats uri /admin?stats

frontend main
    bind *:80
    default_backend app
`
	probe := DetermineProbe(parseSections(cfg))
	if probe.Kind != "stats" || probe.Port != 8404 || probe.Path != "/admin?stats" || probe.Section != "listen stats" {
		t.Errorf("probe = %+v", probe)
	}
}

func TestDetermineProbeDefaultStatsURI(t *testing.T) {
	cfg := `
listen stats
    bind *:8404
    mode http
    stats enable
`
	probe := DetermineProbe(parseSections(cfg))
	if probe.Path != "/haproxy?stats" {
		t.Errorf("default stats uri = %q, want /haproxy?stats (verified against real haproxy)", probe.Path)
	}
}

// With no "mode" anywhere HAProxy runs in TCP mode and ignores both stats
// and monitor-uri (verified), so neither may be chosen as the probe.
func TestDetermineProbeIgnoresHTTPOnlyProbesWithNoModeSet(t *testing.T) {
	cfg := `
frontend main
    bind *:80
    monitor-uri /healthz

listen stats
    bind *:8404
    stats enable
`
	if p := DetermineProbe(parseSections(cfg)); p.Kind != "port" || p.Port != 80 {
		t.Errorf("probe = %+v, want a bare port probe on :80", p)
	}
}

func TestDetermineProbeFallsBackToBarePort(t *testing.T) {
	cfg := `
frontend main
    bind *:80
    default_backend app

backend app
    server app1 10.0.0.1:3000
`
	probe := DetermineProbe(parseSections(cfg))
	if probe.Kind != "port" || probe.Port != 80 || probe.Section != "frontend main" {
		t.Errorf("probe = %+v", probe)
	}
}

func TestDetermineProbeNoneWithoutAnyBind(t *testing.T) {
	cfg := `
global
    log stdout format raw local0

backend app
    server app1 10.0.0.1:3000
`
	probe := DetermineProbe(parseSections(cfg))
	if probe.Kind != "none" {
		t.Errorf("probe = %+v, want none (no frontend/listen with a bind)", probe)
	}
}

func TestParseSectionsIgnoresCommentsAndBlankLines(t *testing.T) {
	cfg := `
# a top comment
frontend main
    # a comment inside the section
    bind *:80

    monitor-uri /healthz
`
	sections := parseSections(cfg)
	if len(sections) != 1 || sections[0].monitorURI != "/healthz" || len(sections[0].binds) != 1 {
		t.Errorf("sections = %+v", sections)
	}
}

func TestParseSectionsCapturesBackendStructure(t *testing.T) {
	cfg := `
frontend main
    bind *:80
    mode http
    default_backend app
    use_backend maint if is_maint

backend app
    mode http
    balance roundrobin
    server app1 10.0.0.1:3000 check
    server app2 10.0.0.2:3000 check
`
	sections := parseSections(cfg)
	if len(sections) != 2 {
		t.Fatalf("len(sections) = %d, want 2", len(sections))
	}
	fe := sections[0]
	if fe.mode != "http" || fe.defaultBackend != "app" || len(fe.useBackends) != 1 || fe.useBackends[0] != "maint if is_maint" {
		t.Errorf("frontend section = %+v", fe)
	}
	be := sections[1]
	if be.balance != "roundrobin" || len(be.servers) != 2 || be.servers[0] != "app1 10.0.0.1:3000 check" {
		t.Errorf("backend section = %+v", be)
	}
	view := be.view("http")
	if view.Balance != "roundrobin" || len(view.Servers) != 2 {
		t.Errorf("view() = %+v", view)
	}
}

func TestTestCommand(t *testing.T) {
	got := testCommand([]string{"/etc/haproxy/haproxy.cfg", "/etc/haproxy/conf.d"})
	want := "haproxy -c -f '/etc/haproxy/haproxy.cfg' -f '/etc/haproxy/conf.d'"
	if got != want {
		t.Errorf("testCommand = %q, want %q", got, want)
	}
}

func TestFirstLine(t *testing.T) {
	got := firstLine("HAProxy version 2.9.7-1 2024/04/23 - https://haproxy.org/\nStatus: stable\n")
	if got != "2.9.7-1 2024/04/23 - https://haproxy.org/" {
		t.Errorf("firstLine = %q", got)
	}
}

func TestLastLinePicksTheAlertLine(t *testing.T) {
	out := "[NOTICE] some startup notice\n[ALERT] config : parsing [/etc/haproxy/haproxy.cfg:8] : unknown keyword 'nope'\nsome trailing line"
	if got := lastLine(out); got != "[ALERT] config : parsing [/etc/haproxy/haproxy.cfg:8] : unknown keyword 'nope'" {
		t.Errorf("lastLine = %q", got)
	}
}

// realStatsCSV is an unedited capture of "curl http://.../haproxy?stats;csv"
// against a real haproxy:lts-alpine (version 3.4.5) container, with one
// backend of two servers — app1 reachable, app2 with nothing listening on
// its port — plus the stats listener's own frontend/backend rows. Used
// verbatim rather than a hand-trimmed fixture so the test exercises the
// real, oddly-wide (150+ column, several blank) row shape rather than a
// tidied-up approximation of it.
const realStatsCSV = `# pxname,svname,qcur,qmax,scur,smax,slim,stot,bin,bout,dreq,dresp,ereq,econ,eresp,wretr,wredis,status,weight,act,bck,chkfail,chkdown,lastchg,downtime,qlimit,pid,iid,sid,throttle,lbtot,tracked,type,rate,rate_lim,rate_max,check_status,check_code,check_duration,hrsp_1xx,hrsp_2xx,hrsp_3xx,hrsp_4xx,hrsp_5xx,hrsp_other,hanafail,req_rate,req_rate_max,req_tot,cli_abrt,srv_abrt,comp_in,comp_out,comp_byp,comp_rsp,lastsess,last_chk,last_agt,qtime,ctime,rtime,ttime,agent_status,agent_code,agent_duration,check_desc,agent_desc,check_rise,check_fall,check_health,agent_rise,agent_fall,agent_health,addr,cookie,mode,algo,conn_rate,conn_rate_max,conn_tot,intercepted,dcon,dses,wrew,connect,reuse,cache_lookups,cache_hits,srv_icur,src_ilim,qtime_max,ctime_max,rtime_max,ttime_max,eint,idle_conn_cur,safe_conn_cur,used_conn_cur,need_conn_est,uweight,agg_server_status,agg_server_check_status,agg_check_status,
main,FRONTEND,,,0,0,524260,0,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,2,0,,,,0,0,0,0,,,,0,0,0,0,0,0,,0,0,0,,,0,0,0,0,,,,,,,,,,,,,,,,,,,,,http,,0,0,0,0,0,0,0,,,0,0,,,,,,,0,,,,,,,,,,0,0,0,0,0,0,0,0,,,0,0,0,0,-,0,0,0,0,0,
app,app1,0,0,0,0,,0,0,0,,0,,0,0,0,0,UP,1,1,0,0,0,4,0,,1,3,1,,0,,2,0,,0,L4OK,,0,0,0,0,0,0,0,,,,0,0,0,,,,,-1,,,0,0,0,0,,,,Layer4 check passed,,2,3,4,,,,,,http,,,,,,,,0,0,0,,,0,,0,0,0,0,0,0,0,0,1,1,,,,0,,,,,,,,,,0,0,0,0,0,-,0,0,0,0,0,
app,app2,0,0,0,0,,0,0,0,,0,,0,0,0,0,DOWN,1,1,0,0,1,3,3,,1,3,2,,0,,2,0,,0,L4CON,,0,0,0,0,0,0,0,,,,0,0,0,,,,,-1,ECONNREFUSED returned by OS (Connection refused),,0,0,0,0,,,,Layer4 connection problem,,2,3,0,,,,,,http,,,,,,,,0,0,0,,,0,,0,0,0,0,0,0,0,0,1,1,,,,0,,,,,,,,,,0,0,0,0,0,-,0,0,0,0,0,
app,BACKEND,0,0,0,0,52426,0,0,0,0,0,,0,0,0,0,UP,1,1,0,,0,4,0,,1,3,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,0,0,0,0,0,0,0,-1,,,0,0,0,0,,,,,,,,,,,,,,http,,,,,,,,0,0,0,0,0,,,0,0,0,0,0,,,,,1,0,0,0,,,,,,,,,,,,0,0,0,0,-,0,0,0,0,0,
stats,FRONTEND,,,1,3,524260,1,0,0,0,0,0,,,,,OPEN,,,,,,,,,1,4,0,,,,0,1,0,1,,,,0,0,0,0,0,0,,1,1,1,,,0,0,0,0,,,,,,,,,,,,,,,,,,,,,http,,1,1,1,1,0,0,0,,,0,0,,,,,,,0,,,,,,,,,,0,1,0,0,0,1,0,0,,,0,0,0,0,-,0,0,0,0,0,
stats,BACKEND,0,0,0,0,52426,0,0,0,0,0,,0,0,0,0,UP,0,0,0,,0,4,,,1,4,0,,0,,1,0,,0,,,,0,0,0,0,0,0,,,,0,0,0,0,0,0,0,0,,,0,0,0,0,,,,,,,,,,,,,,http,,,,,,,,0,0,0,0,0,,,0,0,0,0,0,,,,,0,0,0,0,,,,,,,,,,,,0,0,0,0,-,0,0,0,0,0,
`

func TestParseStatsCSVExtractsOnlyServerRows(t *testing.T) {
	servers := parseStatsCSV(realStatsCSV)
	if len(servers) != 2 {
		t.Fatalf("len(servers) = %d, want 2 (app1, app2 — FRONTEND/BACKEND rows must be excluded for both proxies)", len(servers))
	}

	byName := map[string]BackendServer{}
	for _, s := range servers {
		byName[s.Name] = s
	}

	up, ok := byName["app1"]
	if !ok || up.Backend != "app" || up.Status != "UP" || up.CheckStatus != "L4OK" || up.CheckDesc != "Layer4 check passed" {
		t.Errorf("app1 = %+v", up)
	}
	down, ok := byName["app2"]
	if !ok || down.Backend != "app" || down.Status != "DOWN" || down.CheckStatus != "L4CON" ||
		down.LastCheck != "ECONNREFUSED returned by OS (Connection refused)" || down.Downtime != "3" {
		t.Errorf("app2 = %+v", down)
	}
}

func TestParseStatsCSVEmptyOnGarbage(t *testing.T) {
	if got := parseStatsCSV("not a csv at all"); len(got) != 0 {
		t.Errorf("parseStatsCSV on a header-only non-matching line = %+v, want none", got)
	}
	if got := parseStatsCSV(""); got != nil {
		t.Errorf("parseStatsCSV(\"\") = %+v, want nil", got)
	}
}

func TestStatsEndpointFindsEnabledStatsPageIndependentOfProbe(t *testing.T) {
	cfg := `
defaults
    mode http

frontend main
    bind *:80
    monitor-uri /healthz

listen stats
    bind *:8404
    stats enable
    stats uri /admin?stats
`
	port, uri, ok := statsEndpoint(parseSections(cfg))
	if !ok || port != 8404 || uri != "/admin?stats" {
		t.Errorf("statsEndpoint = %d, %q, %v", port, uri, ok)
	}

	if _, _, ok := statsEndpoint(parseSections("frontend main\n    bind *:80\n")); ok {
		t.Error("statsEndpoint should report false when nothing enables stats")
	}
}

// HAProxy silently ignores "stats" in TCP mode, so such a section must not
// count — whether TCP comes from the section itself or its defaults.
func TestStatsEndpointIgnoresTCPMode(t *testing.T) {
	inherited := `
defaults
    mode tcp

listen stats
    bind 127.0.0.1:8404
    stats enable
`
	if _, _, ok := statsEndpoint(parseSections(inherited)); ok {
		t.Error("stats under tcp defaults counted as enabled")
	}

	overridden := inherited + "    mode http\n"
	if port, _, ok := statsEndpoint(parseSections(overridden)); !ok || port != 8404 {
		t.Errorf("stats with its own mode http under tcp defaults = %d, %v; want 8404, true", port, ok)
	}
}

// Apply's guard clauses run before it ever touches its *sshexec.Client, so
// they're safe to exercise with a nil client/container — everything past
// them needs a real SSH+Docker target and is instead verified by hand
// against a live container (see the session notes: a broken config leaves
// the real file's checksum unchanged and exits 1; a valid one exits 0).
func TestApplyRejectsEmptyChanges(t *testing.T) {
	if err := Apply(nil, nil, nil, io.Discard); err == nil {
		t.Error("Apply with no changes should return an error")
	}
}

func TestApplyRejectsRelativePaths(t *testing.T) {
	err := Apply(nil, nil, []Change{{Path: "relative/haproxy.cfg", Content: "x"}}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "not an absolute path") {
		t.Errorf("Apply with a relative path = %v, want an absolute-path error", err)
	}
}
