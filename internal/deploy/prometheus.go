package deploy

import (
	"fmt"
	"strings"
)

// PrometheusStackConfig holds what's needed to bring up a monitoring stack —
// Prometheus, Grafana, Alertmanager, and node_exporter — wired together out
// of the box: Prometheus scrapes itself, node_exporter, and Alertmanager;
// alerts route to Alertmanager; Grafana is pre-provisioned with Prometheus
// as a datasource. All four run from their official images. Prometheus reads
// its config from a folder on the host (see SeedFilesFor); Grafana's
// datasource uses Compose's inline "configs".
type PrometheusStackConfig struct {
	PrometheusPort   int // host port -> container 9090
	GrafanaPort      int // host port -> container 3000
	AlertmanagerPort int // host port -> container 9093
	NodeExporterPort int // host port -> container 9100

	ImageTag string // shared tag across all four official images, e.g. "latest"

	GrafanaAdminPassword string
	RetentionTime        string // Prometheus --storage.tsdb.retention.time, e.g. "15d"

	DeployDir       string
	ContainerPrefix string // containers named "<prefix>-prometheus", "<prefix>-grafana", ...

	// Networks are extra docker networks every container joins, created
	// fresh in this compose file (not assumed to pre-exist).
	Networks []string

	// ExtraVolumes are additional volume mounts on the Prometheus
	// container specifically, in compose's "host:container[:ro]" form.
	ExtraVolumes []string

	// PrometheusCPULimit and PrometheusMemoryLimit cap Prometheus's own
	// resource usage, e.g. "2" and "4g" — the component in this stack
	// most likely to need sizing. Grafana, Alertmanager, and
	// node_exporter are lightweight and left unbounded.
	PrometheusCPULimit    string
	PrometheusMemoryLimit string
}

func (c PrometheusStackConfig) ComposeYAML() string {
	prom := c.ContainerPrefix + "-prometheus"
	grafana := c.ContainerPrefix + "-grafana"
	alertmanager := c.ContainerPrefix + "-alertmanager"
	nodeExporter := c.ContainerPrefix + "-node-exporter"

	var b strings.Builder
	b.WriteString("services:\n")

	b.WriteString("  prometheus:\n")
	fmt.Fprintf(&b, "    image: prom/prometheus:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", prom)
	b.WriteString("    restart: always\n")
	b.WriteString("    command:\n")
	b.WriteString("      - '--config.file=/etc/prometheus/prometheus.yml'\n")
	b.WriteString("      - '--storage.tsdb.path=/prometheus'\n")
	fmt.Fprintf(&b, "      - '--storage.tsdb.retention.time=%s'\n", c.RetentionTime)
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:9090'\n", c.PrometheusPort)
	b.WriteString("    volumes:\n")
	// Config lives in a folder on the host rather than inline in this
	// file, so the panel's rule management can edit it and the edits
	// survive a redeploy. SeedFilesFor writes the starting files.
	fmt.Fprintf(&b, "      - '%s/%s:/etc/prometheus'\n", c.DeployDir, PrometheusConfigFolder)
	fmt.Fprintf(&b, "      - '%s/prometheus-data:/prometheus'\n", c.DeployDir)
	for _, v := range c.ExtraVolumes {
		fmt.Fprintf(&b, "      - '%s'\n", v)
	}
	if c.PrometheusCPULimit != "" {
		fmt.Fprintf(&b, "    cpus: '%s'\n", c.PrometheusCPULimit)
	}
	if c.PrometheusMemoryLimit != "" {
		fmt.Fprintf(&b, "    mem_limit: %s\n", c.PrometheusMemoryLimit)
	}
	// Verified against a real container: the prom/prometheus image has
	// wget (busybox) but no curl.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"wget\", \"--spider\", \"-q\", \"http://localhost:9090/-/healthy\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 30s\n")
	writeNetworksRef(&b, c.Networks)

	b.WriteString("  grafana:\n")
	fmt.Fprintf(&b, "    image: grafana/grafana:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", grafana)
	b.WriteString("    restart: always\n")
	b.WriteString("    depends_on:\n")
	b.WriteString("      - prometheus\n")
	b.WriteString("    environment:\n")
	fmt.Fprintf(&b, "      - GF_SECURITY_ADMIN_PASSWORD=%s\n", c.GrafanaAdminPassword)
	b.WriteString("    configs:\n")
	b.WriteString("      - source: grafana_datasource\n")
	b.WriteString("        target: /etc/grafana/provisioning/datasources/prometheus.yml\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:3000'\n", c.GrafanaPort)
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/grafana-data:/var/lib/grafana'\n", c.DeployDir)
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD-SHELL\", \"curl -f http://localhost:3000/api/health || exit 1\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 30s\n")
	writeNetworksRef(&b, c.Networks)

	b.WriteString("  alertmanager:\n")
	fmt.Fprintf(&b, "    image: prom/alertmanager:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", alertmanager)
	b.WriteString("    restart: always\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:9093'\n", c.AlertmanagerPort)
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - '%s/alertmanager-data:/alertmanager'\n", c.DeployDir)
	// Verified: prom/alertmanager also has wget but no curl.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"wget\", \"--spider\", \"-q\", \"http://localhost:9093/-/healthy\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 15s\n")
	writeNetworksRef(&b, c.Networks)

	b.WriteString("  node-exporter:\n")
	fmt.Fprintf(&b, "    image: prom/node-exporter:%s\n", c.ImageTag)
	fmt.Fprintf(&b, "    container_name: %s\n", nodeExporter)
	b.WriteString("    restart: always\n")
	b.WriteString("    pid: host\n")
	b.WriteString("    ports:\n")
	fmt.Fprintf(&b, "      - '%d:9100'\n", c.NodeExporterPort)
	b.WriteString("    volumes:\n")
	b.WriteString("      - '/proc:/host/proc:ro'\n")
	b.WriteString("      - '/sys:/host/sys:ro'\n")
	b.WriteString("      - '/:/rootfs:ro'\n")
	b.WriteString("    command:\n")
	b.WriteString("      - '--path.procfs=/host/proc'\n")
	b.WriteString("      - '--path.sysfs=/host/sys'\n")
	b.WriteString("      - '--path.rootfs=/rootfs'\n")
	b.WriteString("      - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)'\n")
	// Verified: prom/node-exporter also has wget but no curl.
	b.WriteString("    healthcheck:\n")
	b.WriteString("      test: [\"CMD\", \"wget\", \"--spider\", \"-q\", \"http://localhost:9100/\"]\n")
	b.WriteString("      interval: 30s\n")
	b.WriteString("      timeout: 10s\n")
	b.WriteString("      retries: 5\n")
	b.WriteString("      start_period: 15s\n")
	writeNetworksRef(&b, c.Networks)

	b.WriteString("\nconfigs:\n")
	b.WriteString("  grafana_datasource:\n")
	b.WriteString("    content: |\n")
	b.WriteString("      apiVersion: 1\n")
	b.WriteString("      datasources:\n")
	b.WriteString("        - name: Prometheus\n")
	b.WriteString("          type: prometheus\n")
	b.WriteString("          access: proxy\n")
	b.WriteString("          url: http://prometheus:9090\n")
	b.WriteString("          isDefault: true\n")

	if len(c.Networks) > 0 {
		b.WriteString("\nnetworks:\n")
		for _, n := range c.Networks {
			fmt.Fprintf(&b, "  %s:\n", n)
		}
	}

	return b.String()
}

func writeNetworksRef(b *strings.Builder, networks []string) {
	if len(networks) == 0 {
		return
	}
	b.WriteString("    networks:\n")
	for _, n := range networks {
		fmt.Fprintf(b, "      - %s\n", n)
	}
}

// PrometheusConfigFolder is the folder under the deploy directory holding
// Prometheus's config and rule files, mounted at /etc/prometheus.
const PrometheusConfigFolder = "prometheus-config"

// DefaultPrometheusConfig is the prometheus.yml a new stack starts with:
// scrape Prometheus itself, node_exporter and Alertmanager, send alerts to
// Alertmanager, and load every rule file under rules/.
const DefaultPrometheusConfig = `global:
  scrape_interval: 15s
  evaluation_interval: 15s

alerting:
  alertmanagers:
    - static_configs:
        - targets: ['alertmanager:9093']

rule_files:
  - rules/*.yml

scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
  - job_name: 'node-exporter'
    static_configs:
      - targets: ['node-exporter:9100']
  - job_name: 'alertmanager'
    static_configs:
      - targets: ['alertmanager:9093']
`

// DefaultAlertRules is the starting rule file: one alert for any scrape
// target that stops responding.
const DefaultAlertRules = `groups:
  - name: availability
    rules:
      - alert: InstanceDown
        expr: up == 0
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: '{{ $labels.instance }} is down'
          description: '{{ $labels.job }} target {{ $labels.instance }} has not answered for 5 minutes.'
`
