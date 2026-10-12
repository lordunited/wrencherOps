// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

// Package servers watches the servers themselves, beside the stacks
// deployed on them: who is logged in and who failed to, disks, kernel
// errors, connections and load — read over one SSH connection per server —
// and raises alerts from rules you manage.
package servers

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot is everything one check read from a server.
type Snapshot struct {
	CollectedAt   time.Time `json:"collectedAt"`
	Hostname      string    `json:"hostname"`
	OS            string    `json:"os"`
	Kernel        string    `json:"kernel"`
	CPUs          int       `json:"cpus"`
	Load1         float64   `json:"load1"`
	Load5         float64   `json:"load5"`
	Load15        float64   `json:"load15"`
	UptimeSeconds float64   `json:"uptimeSeconds"`
	Memory        Memory    `json:"memory"`
	Disks         []Disk    `json:"disks"`
	// Sessions are the users logged in right now (who).
	Sessions []Session `json:"sessions"`
	// Logins are successful SSH logins in the last 24 hours, newest first.
	Logins []AuthEvent `json:"logins"`
	// PanelAddress is the address the panel connects from, as the server
	// sees it; its own logins in Logins are marked Panel.
	PanelAddress string       `json:"panelAddress,omitempty"`
	Failed       FailedLogins `json:"failed"`
	// AuthSource is where SSH logins were read from ("journal", a log
	// file's path), or "" when none could be read.
	AuthSource   string       `json:"authSource"`
	KernelErrors KernelErrors `json:"kernelErrors"`
	Connections  Connections  `json:"connections"`
	Notes        []string     `json:"notes,omitempty"`
}

type Memory struct {
	TotalBytes     int64   `json:"totalBytes"`
	AvailableBytes int64   `json:"availableBytes"`
	UsedPercent    float64 `json:"usedPercent"`
	SwapTotalBytes int64   `json:"swapTotalBytes"`
	SwapUsedBytes  int64   `json:"swapUsedBytes"`
}

type Disk struct {
	Filesystem  string  `json:"filesystem"`
	Mount       string  `json:"mount"`
	SizeBytes   int64   `json:"sizeBytes"`
	UsedBytes   int64   `json:"usedBytes"`
	AvailBytes  int64   `json:"availBytes"`
	UsedPercent float64 `json:"usedPercent"`
	// InodesUsedPercent is -1 when the filesystem doesn't report inodes.
	InodesUsedPercent float64 `json:"inodesUsedPercent"`
}

type Session struct {
	User  string `json:"user"`
	TTY   string `json:"tty"`
	Since string `json:"since"`
	From  string `json:"from"`
}

type AuthEvent struct {
	Time   time.Time `json:"time"`
	User   string    `json:"user"`
	From   string    `json:"from"`
	Method string    `json:"method,omitempty"`
	// Invalid marks a user name that doesn't exist on the server.
	Invalid bool `json:"invalid,omitempty"`
	// Panel marks the panel's own logins (its checks), by the address it
	// connects from and the user it logs in as.
	Panel bool `json:"panel,omitempty"`
}

type Count struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type FailedLogins struct {
	LastHour   int         `json:"lastHour"`
	Last24h    int         `json:"last24h"`
	TopUsers   []Count     `json:"topUsers"`
	TopSources []Count     `json:"topSources"`
	Recent     []AuthEvent `json:"recent"`
}

type KernelLine struct {
	// Time is when it was logged, worked out from the seconds-since-boot
	// stamp; nil when the line has none.
	Time    *time.Time `json:"time,omitempty"`
	Message string     `json:"message"`
}

type KernelErrors struct {
	LastHour int          `json:"lastHour"`
	Total    int          `json:"total"`
	Recent   []KernelLine `json:"recent"`
	// Readable is false when the kernel log couldn't be read at all.
	Readable bool `json:"readable"`
}

type Connections struct {
	Established int     `json:"established"`
	Listen      int     `json:"listen"`
	TimeWait    int     `json:"timeWait"`
	CloseWait   int     `json:"closeWait"`
	SynRecv     int     `json:"synRecv"`
	Total       int     `json:"total"`
	TopPeers    []Count `json:"topPeers"`
	Readable    bool    `json:"readable"`
}

// Section markers in the collection script's output.
const (
	mClient = "@@SRV-CLIENT@@"
	mSys    = "@@SRV-SYS@@"
	mMem    = "@@SRV-MEM@@"
	mDF     = "@@SRV-DF@@"
	mDFI    = "@@SRV-DFI@@"
	mWho    = "@@SRV-WHO@@"
	mAuth   = "@@SRV-AUTH@@"
	mDmesg  = "@@SRV-DMESG@@"
	mConn   = "@@SRV-CONN@@"
	mEnd    = "@@SRV-END@@"
)

// collectScript reads everything a Snapshot holds in one go. It runs as
// root (failed logins, the kernel log and the journal usually need it) and
// sticks to POSIX sh and tools found on any Linux, with fallbacks for
// minimal systems (busybox: no ss, no dmesg --level, no journal).
// SSH logins come from the journal's auth facility for the last 24 hours,
// else from the classic auth log file.
const collectScript = `
echo '` + mSys + `'
hostname 2>/dev/null || cat /proc/sys/kernel/hostname
uname -r 2>/dev/null
( . /etc/os-release 2>/dev/null && echo "$PRETTY_NAME" ) || echo
nproc 2>/dev/null || grep -c ^processor /proc/cpuinfo
cat /proc/loadavg
cat /proc/uptime
date +%s
date +%z
echo '` + mMem + `'
cat /proc/meminfo
echo '` + mDF + `'
df -P 2>/dev/null
echo '` + mDFI + `'
df -P -i 2>/dev/null
echo '` + mWho + `'
who 2>/dev/null
echo '` + mAuth + `'
J=""
if command -v journalctl >/dev/null 2>&1; then
  J=$(journalctl -q --no-pager --since -24h -o short-unix SYSLOG_FACILITY=4 SYSLOG_FACILITY=10 2>/dev/null | grep -E 'sshd.*(Failed |Accepted |Invalid user )' | tail -n 5000)
fi
if [ -n "$J" ]; then
  echo '#journal'
  printf '%s\n' "$J"
else
  for f in /var/log/auth.log /var/log/secure /var/log/messages; do
    if [ -f "$f" ]; then
      echo "#file $f"
      grep -hE 'sshd.*(Failed |Accepted |Invalid user )' "$f" 2>/dev/null | tail -n 5000
      break
    fi
  done
fi
echo '` + mDmesg + `'
if dmesg --level=emerg,alert,crit,err >/dev/null 2>&1; then
  echo '#level'
  dmesg --level=emerg,alert,crit,err 2>/dev/null | tail -n 300
elif dmesg >/dev/null 2>&1; then
  echo '#grep'
  dmesg 2>/dev/null | grep -iE 'error|fail|critical|panic|oom|segfault|i/o|corrupt|hung task' | tail -n 300
fi
echo '` + mConn + `'
if command -v ss >/dev/null 2>&1; then
  echo '#ss'
  ss -Htan 2>/dev/null
elif command -v netstat >/dev/null 2>&1; then
  echo '#netstat'
  netstat -tan 2>/dev/null
fi
echo '` + mEnd + `'
`

// parse turns the script's output into a Snapshot. loginUser is the user
// the panel logged in as.
func parse(out, loginUser string) *Snapshot {
	sec := sections(out)
	s := &Snapshot{CollectedAt: time.Now().UTC()}
	if f := strings.Fields(sec[mClient]); len(f) > 0 {
		s.PanelAddress = f[0]
	}

	sys := lines(sec[mSys])
	at := func(i int) string {
		if i < len(sys) {
			return strings.TrimSpace(sys[i])
		}
		return ""
	}
	s.Hostname, s.Kernel, s.OS = at(0), at(1), at(2)
	s.CPUs, _ = strconv.Atoi(at(3))
	if s.CPUs < 1 {
		s.CPUs = 1
	}
	if f := strings.Fields(at(4)); len(f) >= 3 {
		s.Load1, _ = strconv.ParseFloat(f[0], 64)
		s.Load5, _ = strconv.ParseFloat(f[1], 64)
		s.Load15, _ = strconv.ParseFloat(f[2], 64)
	}
	if f := strings.Fields(at(5)); len(f) >= 1 {
		s.UptimeSeconds, _ = strconv.ParseFloat(f[0], 64)
	}
	now := time.Now()
	if n, err := strconv.ParseInt(at(6), 10, 64); err == nil {
		now = time.Unix(n, 0)
	}
	loc := zoneFrom(at(7))

	s.Memory = parseMeminfo(sec[mMem])
	s.Disks = parseDF(sec[mDF], sec[mDFI])
	s.Sessions = parseWho(sec[mWho])
	s.AuthSource, s.Logins, s.Failed = parseAuth(sec[mAuth], now, loc)
	for i := range s.Logins {
		if s.PanelAddress != "" && s.Logins[i].From == s.PanelAddress && s.Logins[i].User == loginUser {
			s.Logins[i].Panel = true
		}
	}
	if s.AuthSource == "" {
		s.Notes = append(s.Notes, "SSH logins couldn't be read: no journal entries and no auth log file (/var/log/auth.log, /var/log/secure)")
	}
	s.KernelErrors = parseDmesg(sec[mDmesg], now, s.UptimeSeconds)
	if !s.KernelErrors.Readable {
		s.Notes = append(s.Notes, "the kernel log (dmesg) couldn't be read")
	}
	s.Connections = parseConnections(sec[mConn])
	if !s.Connections.Readable {
		s.Notes = append(s.Notes, "connections couldn't be read: neither ss nor netstat is installed")
	}
	return s
}

func sections(out string) map[string]string {
	markers := []string{mClient, mSys, mMem, mDF, mDFI, mWho, mAuth, mDmesg, mConn, mEnd}
	sec := map[string]string{}
	for i, m := range markers[:len(markers)-1] {
		start := strings.Index(out, m+"\n")
		if start < 0 {
			continue
		}
		body := out[start+len(m)+1:]
		if end := strings.Index(body, markers[i+1]+"\n"); end >= 0 {
			body = body[:end]
		}
		sec[m] = body
	}
	return sec
}

func lines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// zoneFrom turns `date +%z` ("+0330") into a fixed zone, for log files that
// stamp lines in the server's local time without a zone.
func zoneFrom(z string) *time.Location {
	if len(z) == 5 && (z[0] == '+' || z[0] == '-') {
		h, err1 := strconv.Atoi(z[1:3])
		m, err2 := strconv.Atoi(z[3:5])
		if err1 == nil && err2 == nil {
			off := h*3600 + m*60
			if z[0] == '-' {
				off = -off
			}
			return time.FixedZone(z, off)
		}
	}
	return time.UTC
}

func parseMeminfo(s string) Memory {
	kb := map[string]int64{}
	for _, l := range lines(s) {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		f := strings.Fields(v)
		if len(f) > 0 {
			n, _ := strconv.ParseInt(f[0], 10, 64)
			kb[k] = n * 1024
		}
	}
	m := Memory{TotalBytes: kb["MemTotal"], AvailableBytes: kb["MemAvailable"], SwapTotalBytes: kb["SwapTotal"]}
	if _, ok := kb["MemAvailable"]; !ok {
		m.AvailableBytes = kb["MemFree"] + kb["Buffers"] + kb["Cached"]
	}
	m.SwapUsedBytes = kb["SwapTotal"] - kb["SwapFree"]
	if m.TotalBytes > 0 {
		m.UsedPercent = round1(100 * float64(m.TotalBytes-m.AvailableBytes) / float64(m.TotalBytes))
	}
	return m
}

// Pseudo and container filesystems aren't disks anyone fills up.
var skipFS = map[string]bool{"tmpfs": true, "devtmpfs": true, "overlay": true, "shm": true, "none": true, "udev": true, "squashfs": true, "efivarfs": true, "cgroup": true, "cgroup2": true, "proc": true, "sysfs": true, "devpts": true, "mqueue": true, "nsfs": true}

func skipMount(fs, mount string) bool {
	if skipFS[fs] {
		return true
	}
	// Single files bind-mounted into a container, not disks.
	switch mount {
	case "/etc/hosts", "/etc/hostname", "/etc/resolv.conf":
		return true
	}
	// System folders and snaps, with everything under them.
	for _, p := range []string{"/dev", "/run", "/sys", "/proc", "/snap"} {
		if mount == p || strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	// Container mounts under a runtime's folder — but a disk mounted at the
	// folder itself is a real disk.
	for _, p := range []string{"/var/lib/docker", "/var/lib/containers", "/var/lib/kubelet"} {
		if strings.HasPrefix(mount, p+"/") {
			return true
		}
	}
	return false
}

// dfRow splits a `df -P` row: the mount point is everything after the 5th
// column (it may contain spaces).
func dfRow(l string) (fs string, nums []string, mount string, ok bool) {
	f := strings.Fields(l)
	if len(f) < 6 || f[0] == "Filesystem" {
		return "", nil, "", false
	}
	return f[0], f[1:5], strings.Join(f[5:], " "), true
}

func parseDF(df, dfi string) []Disk {
	inodes := map[string]float64{}
	for _, l := range lines(dfi) {
		_, nums, mount, ok := dfRow(l)
		if !ok {
			continue
		}
		if p, err := strconv.ParseFloat(strings.TrimSuffix(nums[3], "%"), 64); err == nil {
			inodes[mount] = p
		}
	}
	var disks []Disk
	seen := map[string]bool{}
	for _, l := range lines(df) {
		fs, nums, mount, ok := dfRow(l)
		if !ok || skipMount(fs, mount) || seen[mount] {
			continue
		}
		size, err1 := strconv.ParseInt(nums[0], 10, 64)
		used, err2 := strconv.ParseInt(nums[1], 10, 64)
		avail, err3 := strconv.ParseInt(nums[2], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil || size == 0 {
			continue
		}
		seen[mount] = true
		d := Disk{Filesystem: fs, Mount: mount, SizeBytes: size * 1024, UsedBytes: used * 1024, AvailBytes: avail * 1024, InodesUsedPercent: -1}
		// df's own figure: used / (used + available), as it rounds it.
		if p, err := strconv.ParseFloat(strings.TrimSuffix(nums[3], "%"), 64); err == nil {
			d.UsedPercent = p
		} else if used+avail > 0 {
			d.UsedPercent = round1(100 * float64(used) / float64(used+avail))
		}
		if p, ok := inodes[mount]; ok {
			d.InodesUsedPercent = p
		}
		disks = append(disks, d)
	}
	sort.Slice(disks, func(i, j int) bool { return disks[i].Mount < disks[j].Mount })
	return disks
}

var parenRe = regexp.MustCompile(`\(([^)]*)\)\s*$`)

func parseWho(s string) []Session {
	var out []Session
	for _, l := range lines(s) {
		f := strings.Fields(l)
		if len(f) < 2 || f[0] == "USER" || f[0] == "NAME" {
			continue
		}
		sess := Session{User: f[0], TTY: f[1]}
		rest := strings.TrimSpace(strings.Join(f[2:], " "))
		if m := parenRe.FindStringSubmatch(rest); m != nil {
			sess.From = m[1]
			rest = strings.TrimSpace(strings.TrimSuffix(rest, m[0]))
		}
		sess.Since = rest
		out = append(out, sess)
	}
	return out
}

var (
	failedRe   = regexp.MustCompile(`Failed (\S+) for (invalid user )?(\S*) from (\S+) port \d+`)
	acceptedRe = regexp.MustCompile(`Accepted (\S+) for (\S+) from (\S+) port \d+`)
	invalidRe  = regexp.MustCompile(`Invalid user (\S*) from (\S+)(?: port \d+)?`)
	pidRe      = regexp.MustCompile(`sshd(?:-session)?\[(\d+)\]`)
	rfc3339Re  = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2}))\s`)
	syslogRe   = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2} \d{2}:\d{2}:\d{2})\s`)
)

// authTime reads a log line's timestamp: the journal's unix seconds, an
// RFC 3339 stamp (rsyslog's modern format), or a classic syslog stamp in
// the server's local time, which has no year.
func authTime(l string, journal bool, now time.Time, loc *time.Location) (time.Time, bool) {
	if journal {
		f, _, _ := strings.Cut(l, " ")
		if secs, err := strconv.ParseFloat(f, 64); err == nil {
			return time.Unix(int64(secs), 0), true
		}
		return time.Time{}, false
	}
	if m := rfc3339Re.FindStringSubmatch(l); m != nil {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999-0700"} {
			if t, err := time.Parse(layout, m[1]); err == nil {
				return t, true
			}
		}
	}
	if m := syslogRe.FindStringSubmatch(l); m != nil {
		t, err := time.ParseInLocation("2006 Jan _2 15:04:05", strconv.Itoa(now.In(loc).Year())+" "+strings.Join(strings.Fields(m[1]), " "), loc)
		if err != nil {
			t, err = time.ParseInLocation("2006 Jan 2 15:04:05", strconv.Itoa(now.In(loc).Year())+" "+strings.Join(strings.Fields(m[1]), " "), loc)
		}
		if err == nil {
			if t.After(now.Add(24 * time.Hour)) {
				t = t.AddDate(-1, 0, 0) // December's lines read in January
			}
			return t, true
		}
	}
	return time.Time{}, false
}

// parseAuth reads sshd's log lines from the last 24 hours. Each "Failed"
// line is a failed attempt; an "Invalid user" line counts too — a key-only
// server logs nothing else for a made-up name — unless the same sshd
// process also logged a "Failed ... for invalid user" for it.
func parseAuth(s string, now time.Time, loc *time.Location) (source string, logins []AuthEvent, failed FailedLogins) {
	ls := lines(s)
	if len(ls) == 0 {
		return "", nil, failed
	}
	switch {
	case ls[0] == "#journal":
		source = "journal"
	case strings.HasPrefix(ls[0], "#file "):
		source = strings.TrimPrefix(ls[0], "#file ")
	default:
		return "", nil, failed
	}
	journal := source == "journal"
	dayAgo, hourAgo := now.Add(-24*time.Hour), now.Add(-time.Hour)

	type attempt struct {
		ev      AuthEvent
		pid     string
		invalid bool // an "Invalid user" line
	}
	var attempts []attempt
	failedInvalidPID := map[string]bool{}
	for _, l := range ls[1:] {
		t, ok := authTime(l, journal, now, loc)
		if !ok || t.Before(dayAgo) {
			continue
		}
		pid := ""
		if m := pidRe.FindStringSubmatch(l); m != nil {
			pid = m[1]
		}
		switch {
		case acceptedRe.MatchString(l):
			m := acceptedRe.FindStringSubmatch(l)
			logins = append(logins, AuthEvent{Time: t, User: m[2], From: m[3], Method: m[1]})
		case failedRe.MatchString(l):
			m := failedRe.FindStringSubmatch(l)
			if m[2] != "" {
				failedInvalidPID[pid] = true
			}
			attempts = append(attempts, attempt{ev: AuthEvent{Time: t, User: m[3], From: m[4], Method: m[1], Invalid: m[2] != ""}, pid: pid})
		case invalidRe.MatchString(l):
			m := invalidRe.FindStringSubmatch(l)
			attempts = append(attempts, attempt{ev: AuthEvent{Time: t, User: m[1], From: m[2], Invalid: true}, pid: pid, invalid: true})
		}
	}

	users, sources := map[string]int{}, map[string]int{}
	var recent []AuthEvent
	for _, a := range attempts {
		if a.invalid && a.pid != "" && failedInvalidPID[a.pid] {
			continue
		}
		failed.Last24h++
		if !a.ev.Time.Before(hourAgo) {
			failed.LastHour++
		}
		users[a.ev.User]++
		sources[a.ev.From]++
		recent = append(recent, a.ev)
	}
	failed.TopUsers, failed.TopSources = top(users, 5), top(sources, 5)
	sort.SliceStable(recent, func(i, j int) bool { return recent[i].Time.After(recent[j].Time) })
	sort.SliceStable(logins, func(i, j int) bool { return logins[i].Time.After(logins[j].Time) })
	failed.Recent = capList(recent, 50)
	return source, capList(logins, 50), failed
}

func top(m map[string]int, n int) []Count {
	out := make([]Count, 0, len(m))
	for k, v := range m {
		out = append(out, Count{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Key < out[j].Key
	})
	return capList(out, n)
}

func capList[T any](s []T, n int) []T {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var dmesgRe = regexp.MustCompile(`^\[\s*(\d+(?:\.\d+)?)\]\s?(.*)$`)

// parseDmesg reads kernel error lines. Their "[seconds since boot]" stamp
// gives when each was logged, so "in the last hour" is exact whatever the
// size of the kernel's ring buffer.
func parseDmesg(s string, now time.Time, uptime float64) KernelErrors {
	ls := lines(s)
	k := KernelErrors{}
	if len(ls) == 0 || (ls[0] != "#level" && ls[0] != "#grep") {
		return k
	}
	k.Readable = true
	boot := now.Add(-time.Duration(uptime * float64(time.Second)))
	var all []KernelLine
	for _, l := range ls[1:] {
		if strings.TrimSpace(l) == "" {
			continue
		}
		line := KernelLine{Message: strings.TrimSpace(l)}
		if m := dmesgRe.FindStringSubmatch(l); m != nil {
			secs, _ := strconv.ParseFloat(m[1], 64)
			t := boot.Add(time.Duration(secs * float64(time.Second))).UTC()
			line.Time, line.Message = &t, strings.TrimSpace(m[2])
			if uptime > 0 && uptime-secs <= 3600 {
				k.LastHour++
			}
		}
		all = append(all, line)
	}
	k.Total = len(all)
	// Newest first, and only the last 50.
	for i := len(all) - 1; i >= 0 && len(k.Recent) < 50; i-- {
		k.Recent = append(k.Recent, all[i])
	}
	return k
}

// parseConnections counts TCP sockets by state, from ss (state first) or
// netstat (state last, underscores), and the peers with the most
// established connections.
func parseConnections(s string) Connections {
	ls := lines(s)
	c := Connections{}
	if len(ls) == 0 || (ls[0] != "#ss" && ls[0] != "#netstat") {
		return c
	}
	c.Readable = true
	isSS := ls[0] == "#ss"
	peers := map[string]int{}
	for _, l := range ls[1:] {
		f := strings.Fields(l)
		var state, peer string
		if isSS {
			if len(f) < 5 {
				continue
			}
			state, peer = f[0], f[4]
		} else {
			if len(f) < 6 || !strings.HasPrefix(f[0], "tcp") {
				continue
			}
			state, peer = strings.ReplaceAll(f[5], "_", "-"), f[4]
		}
		c.Total++
		switch state {
		case "ESTAB", "ESTABLISHED":
			c.Established++
			peers[peerHost(peer)]++
		case "LISTEN":
			c.Listen++
		case "TIME-WAIT":
			c.TimeWait++
		case "CLOSE-WAIT":
			c.CloseWait++
		case "SYN-RECV":
			c.SynRecv++
		}
	}
	c.TopPeers = top(peers, 5)
	return c
}

// peerHost strips the port from "1.2.3.4:5678", "[::1]:22" or
// "::ffff:1.2.3.4:5678".
func peerHost(p string) string {
	if i := strings.LastIndex(p, ":"); i > 0 {
		p = p[:i]
	}
	p = strings.Trim(p, "[]")
	return strings.TrimPrefix(p, "::ffff:")
}

func round1(f float64) float64 {
	return float64(int64(f*10+0.5)) / 10
}
