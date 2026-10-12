// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package service

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

// Lines as `docker inspect` prints them for the format containerPorts uses,
// taken from a real SigNoz stack plus a stopped container and a loopback one.
const inspectOut = "/signoz-signoz-0\ttrue\tsignoz-signoz-0\t{\"8080/tcp\":[{\"HostIp\":\"\",\"HostPort\":\"8080\"}]}\t{\"8080/tcp\":[{\"HostIp\":\"0.0.0.0\",\"HostPort\":\"8080\"},{\"HostIp\":\"::\",\"HostPort\":\"8080\"}]}\n" +
	"/signoz-ingester-1\ttrue\tingester\t{\"4317/tcp\":[{\"HostIp\":\"\",\"HostPort\":\"4317\"}],\"4318/tcp\":[{\"HostIp\":\"\",\"HostPort\":\"4318\"}]}\t{\"4317/tcp\":[{\"HostIp\":\"0.0.0.0\",\"HostPort\":\"4317\"}],\"4318/tcp\":[{\"HostIp\":\"0.0.0.0\",\"HostPort\":\"4318\"}],\"8888/tcp\":null}\n" +
	"/db\tfalse\tdb\t{\"5432/tcp\":[{\"HostIp\":\"\",\"HostPort\":\"5432\"}]}\t{}\n" +
	"/stats\ttrue\thaproxy\t{\"8404/tcp\":[{\"HostIp\":\"127.0.0.1\",\"HostPort\":\"8404\"}]}\t{\"8404/tcp\":[{\"HostIp\":\"127.0.0.1\",\"HostPort\":\"8404\"}]}\n" +
	"/dns\ttrue\tdns\t{\"53/udp\":[{\"HostIp\":\"\",\"HostPort\":\"53\"}]}\t{\"53/udp\":[{\"HostIp\":\"0.0.0.0\",\"HostPort\":\"53\"}]}\n"

func TestParsePorts(t *testing.T) {
	ports := parsePorts(inspectOut)
	var got []string
	for _, p := range ports {
		got = append(got, p.Service+" "+p.HostIP+":"+p.HostPort+"->"+p.ContainerPort+"/"+p.Protocol+" "+p.State)
	}
	want := []string{
		"dns :53->53/udp ",
		"ingester :4317->4317/tcp ",
		"ingester :4318->4318/tcp ",
		"db :5432->5432/tcp stopped",
		"signoz-signoz-0 :8080->8080/tcp ",
		"haproxy 127.0.0.1:8404->8404/tcp ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestProbePorts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	open := ln.Addr().(*net.TCPAddr).Port
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	closed := ln2.Addr().(*net.TCPAddr).Port
	ln2.Close()
	defer ln.Close()

	ports := []PortStatus{
		{HostPort: itoa(open), Protocol: "tcp"},
		{HostPort: itoa(closed), Protocol: "tcp"},
		{HostPort: "53", Protocol: "udp"},
		{HostIP: "127.0.0.1", HostPort: "8404", Protocol: "tcp"},
		{HostPort: "5432", Protocol: "tcp", State: "stopped"},
	}
	probePorts("127.0.0.1", ports)
	for i, want := range []string{"open", "closed", "published", "local", "stopped"} {
		if ports[i].State != want {
			t.Errorf("port %d: state %q (%s), want %q", i, ports[i].State, ports[i].Detail, want)
		}
	}
	if !strings.Contains(ports[1].Detail, "refused") {
		t.Errorf("closed detail = %q", ports[1].Detail)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
