// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package servers

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

// An Ubuntu server: journal, ss, dmesg --level, GNU df/who.
func ubuntuOutput(now time.Time) string {
	u := func(d time.Duration) string { return strconv.FormatInt(now.Add(-d).Unix(), 10) + ".123456" }
	return strings.Join([]string{
		mClient, "192.168.1.20",
		mSys, "web-1", "6.8.0-45-generic", "Ubuntu 24.04.1 LTS", "4", "3.20 9.00 2.10 3/412 9921", "7200.55 25000.10", strconv.FormatInt(now.Unix(), 10), "+0330",
		mMem, "MemTotal:        8000000 kB", "MemFree:          200000 kB", "MemAvailable:     400000 kB", "SwapTotal:       2000000 kB", "SwapFree:        1000000 kB",
		mDF,
		"Filesystem     1024-blocks      Used Available Capacity Mounted on",
		"/dev/sda1         100000000  96000000   4000000      96% /",
		"tmpfs               800000         0    800000       0% /run",
		"/dev/sdb1         200000000  20000000 180000000      10% /data disk",
		"overlay           100000000  96000000   4000000      96% /var/lib/docker/overlay2/abc/merged",
		"/dev/loop0             1000      1000         0     100% /snap/core/1",
		mDFI,
		"Filesystem      Inodes   IUsed   IFree IUse% Mounted on",
		"/dev/sda1      6000000 5880000  120000   98% /",
		"/dev/sdb1     12000000   10000 11990000    1% /data disk",
		mWho,
		"root     pts/0        2026-10-02 10:00 (10.0.0.5)",
		"ansiman  pts/1        2026-10-02 11:30 (192.168.1.20)",
		mAuth, "#journal",
		u(10*time.Minute) + " web-1 sshd[100]: Failed password for root from 45.1.1.1 port 4000 ssh2",
		u(9*time.Minute) + " web-1 sshd[100]: Failed password for root from 45.1.1.1 port 4000 ssh2",
		u(8*time.Minute) + " web-1 sshd[101]: Invalid user admin from 45.1.1.1 port 4001",
		u(8*time.Minute) + " web-1 sshd[101]: Failed password for invalid user admin from 45.1.1.1 port 4001 ssh2",
		u(7*time.Minute) + " web-1 sshd-session[102]: Invalid user oracle from 61.2.2.2 port 5000",
		u(3*time.Hour) + " web-1 sshd[103]: Failed password for ubuntu from 61.2.2.2 port 5001 ssh2",
		u(30*time.Hour) + " web-1 sshd[104]: Failed password for root from 9.9.9.9 port 1 ssh2",
		u(5*time.Minute) + " web-1 sshd[105]: Accepted publickey for ansiman from 192.168.1.20 port 6000 ssh2: ED25519 SHA256:x",
		u(2*time.Hour) + " web-1 sshd[106]: Accepted password for root from 10.0.0.5 port 6001 ssh2",
		mDmesg, "#level",
		"[  100.000000] EXT4-fs error (device sda1): old problem",
		"[ 7000.500000] blk_update_request: I/O error, dev sdb, sector 123",
		"[ 7100.000000] Out of memory: Killed process 4242 (java)",
		mConn, "#ss",
		"LISTEN 0 4096 0.0.0.0:22 0.0.0.0:*",
		"ESTAB 0 0 10.0.0.2:22 10.0.0.5:51000",
		"ESTAB 0 0 10.0.0.2:443 [::ffff:8.8.8.8]:40000",
		"ESTAB 0 0 10.0.0.2:443 [::ffff:8.8.8.8]:40001",
		"TIME-WAIT 0 0 10.0.0.2:443 1.1.1.1:3000",
		"CLOSE-WAIT 1 0 10.0.0.2:8080 1.1.1.1:3001",
		mEnd, "",
	}, "\n")
}

func TestParseUbuntu(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	s := parse(ubuntuOutput(now), "ansiman")

	if s.Hostname != "web-1" || s.OS != "Ubuntu 24.04.1 LTS" || s.CPUs != 4 || s.Load5 != 9 || s.UptimeSeconds != 7200.55 {
		t.Errorf("system: %+v", s)
	}
	if s.Memory.UsedPercent != 95 || s.Memory.SwapUsedBytes != 1000000*1024 {
		t.Errorf("memory: %+v", s.Memory)
	}
	if len(s.Disks) != 2 || s.Disks[0].Mount != "/" || s.Disks[0].UsedPercent != 96 || s.Disks[0].InodesUsedPercent != 98 || s.Disks[1].Mount != "/data disk" {
		t.Errorf("disks: %+v", s.Disks)
	}
	if len(s.Sessions) != 2 || s.Sessions[1].User != "ansiman" || s.Sessions[1].From != "192.168.1.20" || s.Sessions[1].Since != "2026-10-02 11:30" {
		t.Errorf("sessions: %+v", s.Sessions)
	}
	// 2 root + 1 admin (its Invalid line folded into its Failed line) +
	// 1 oracle (Invalid only, key-only attempt) in the hour; + ubuntu 3h ago;
	// the 30h-old one is outside the day.
	if s.AuthSource != "journal" || s.Failed.LastHour != 4 || s.Failed.Last24h != 5 {
		t.Errorf("failed: source %q, hour %d, day %d: %+v", s.AuthSource, s.Failed.LastHour, s.Failed.Last24h, s.Failed.Recent)
	}
	if s.Failed.TopSources[0] != (Count{"45.1.1.1", 3}) || s.Failed.TopUsers[0] != (Count{"root", 2}) {
		t.Errorf("top: %+v %+v", s.Failed.TopSources, s.Failed.TopUsers)
	}
	if len(s.Logins) != 2 || s.Logins[0].User != "ansiman" || s.Logins[0].Method != "publickey" || s.Logins[1].User != "root" {
		t.Errorf("logins: %+v", s.Logins)
	}
	// The panel logs in as ansiman from 192.168.1.20: that one is its own.
	if s.PanelAddress != "192.168.1.20" || !s.Logins[0].Panel || s.Logins[1].Panel {
		t.Errorf("panel logins: address %q, %+v", s.PanelAddress, s.Logins)
	}
	// Uptime 7200s: lines at 7000s and 7100s are within the hour, 100s isn't.
	if !s.KernelErrors.Readable || s.KernelErrors.Total != 3 || s.KernelErrors.LastHour != 2 || !strings.HasPrefix(s.KernelErrors.Recent[0].Message, "Out of memory") {
		t.Errorf("kernel: %+v", s.KernelErrors)
	}
	if s.KernelErrors.Recent[0].Time == nil || s.KernelErrors.Recent[0].Time.Sub(now.Add(-100*time.Second)).Abs() > 2*time.Second {
		t.Errorf("kernel line time: %v, want ~100s ago", s.KernelErrors.Recent[0].Time)
	}
	c := s.Connections
	if c.Established != 3 || c.Listen != 1 || c.TimeWait != 1 || c.CloseWait != 1 || c.TopPeers[0] != (Count{"8.8.8.8", 2}) {
		t.Errorf("connections: %+v", c)
	}
	if len(s.Notes) != 0 {
		t.Errorf("notes: %v", s.Notes)
	}
}

// A minimal (busybox) server: a syslog file, netstat, dmesg without --level.
func TestParseBusybox(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	loc := time.FixedZone("+0330", 3*3600+1800)
	stamp := func(d time.Duration) string { return now.Add(-d).In(loc).Format("Jan _2 15:04:05") }
	out := strings.Join([]string{
		mSys, "box", "5.15.0", "Alpine Linux v3.20", "2", "0.10 0.20 0.30 1/50 99", "500.00 900.00", strconv.FormatInt(now.Unix(), 10), "+0330",
		mMem, "MemTotal: 1000 kB", "MemFree: 500 kB",
		mDF, "Filesystem           1024-blocks    Used Available Capacity Mounted on", "/dev/vda1 1000 500 500 50% /",
		mDFI, "Filesystem Inodes Used Available Use% Mounted on", "/dev/vda1 100 10 90 10% /",
		mWho,
		mAuth, "#file /var/log/messages",
		stamp(20*time.Minute) + " box authpriv.info sshd[7]: Failed password for root from 5.5.5.5 port 22 ssh2",
		"2026-10-02T03:04:05.123456+03:30 box sshd[8]: Accepted password for deploy from 6.6.6.6 port 1 ssh2",
		mDmesg, "#grep", "[  480.000000] ata1: I/O error",
		mConn, "#netstat",
		"Active Internet connections (servers and established)",
		"Proto Recv-Q Send-Q Local Address           Foreign Address         State",
		"tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN",
		"tcp        0      0 172.17.0.2:22           172.17.0.1:5000         ESTABLISHED",
		"tcp        0      0 172.17.0.2:22           172.17.0.1:5001         TIME_WAIT",
		mEnd, "",
	}, "\n")
	s := parse(out, "root")
	if s.AuthSource != "/var/log/messages" || s.Failed.LastHour != 1 || s.Failed.Recent[0].User != "root" {
		t.Errorf("failed: %+v", s.Failed)
	}
	if got := s.Failed.Recent[0].Time; got.Sub(now.Add(-20*time.Minute)).Abs() > time.Second {
		t.Errorf("syslog time %v, want %v (server's local time)", got, now.Add(-20*time.Minute))
	}
	if s.Memory.UsedPercent != 50 {
		t.Errorf("memory without MemAvailable: %+v", s.Memory)
	}
	if !s.KernelErrors.Readable || s.KernelErrors.LastHour != 1 {
		t.Errorf("kernel: %+v", s.KernelErrors)
	}
	if s.Connections.Established != 1 || s.Connections.TimeWait != 1 || s.Connections.Listen != 1 {
		t.Errorf("netstat: %+v", s.Connections)
	}
}

// Nothing readable: notes say what's missing rather than showing zeros.
func TestParseNothingReadable(t *testing.T) {
	s := parse(strings.Join([]string{mSys, "x", mMem, mDF, mDFI, mWho, mAuth, mDmesg, mConn, mEnd, ""}, "\n"), "root")
	if s.AuthSource != "" || s.KernelErrors.Readable || s.Connections.Readable || len(s.Notes) != 3 {
		t.Errorf("%+v", s)
	}
}
