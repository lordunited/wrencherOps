// Starting points for common alerts. They assume the exporters people
// usually run next to Prometheus: node_exporter for hosts and the blackbox
// exporter for websites, ports and certificates.

export interface AlertTemplate {
  id: string;
  label: string;
  alert: string;
  expr: string;
  for: string;
  severity: "critical" | "warning" | "info";
  summary: string;
  description: string;
}

export const ALERT_TEMPLATES: AlertTemplate[] = [
  {
    id: "down",
    label: "Target down",
    alert: "InstanceDown",
    expr: "up == 0",
    for: "5m",
    severity: "critical",
    summary: "{{ $labels.instance }} is down",
    description: "{{ $labels.job }} target {{ $labels.instance }} has not answered for 5 minutes.",
  },
  {
    id: "cpu",
    label: "High CPU",
    alert: "HostHighCpu",
    expr: '100 - (avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100) > 85',
    for: "10m",
    severity: "warning",
    summary: "CPU above 85% on {{ $labels.instance }}",
    description: "CPU usage is {{ $value | humanize }}% for 10 minutes.",
  },
  {
    id: "memory",
    label: "High memory",
    alert: "HostHighMemory",
    expr: "(1 - node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes) * 100 > 90",
    for: "10m",
    severity: "warning",
    summary: "Memory above 90% on {{ $labels.instance }}",
    description: "Memory usage is {{ $value | humanize }}%.",
  },
  {
    id: "disk",
    label: "Disk almost full",
    alert: "HostDiskAlmostFull",
    expr: '(1 - node_filesystem_avail_bytes{fstype!~"tmpfs|overlay"} / node_filesystem_size_bytes{fstype!~"tmpfs|overlay"}) * 100 > 85',
    for: "15m",
    severity: "warning",
    summary: "Disk {{ $labels.mountpoint }} above 85% on {{ $labels.instance }}",
    description: "Disk usage is {{ $value | humanize }}%. Free up space before it fills.",
  },
  {
    id: "website",
    label: "Website or port down",
    alert: "ProbeFailed",
    expr: "probe_success == 0",
    for: "2m",
    severity: "critical",
    summary: "{{ $labels.instance }} is not reachable",
    description: "The blackbox check for {{ $labels.instance }} has failed for 2 minutes.",
  },
  {
    id: "ssl",
    label: "SSL certificate expiring",
    alert: "SslCertExpiringSoon",
    expr: "(probe_ssl_earliest_cert_expiry - time()) / 86400 < 14",
    for: "1h",
    severity: "warning",
    summary: "SSL certificate for {{ $labels.instance }} expires soon",
    description: "The certificate expires in {{ $value | humanize }} days.",
  },
  {
    id: "slow",
    label: "Slow website",
    alert: "ProbeSlow",
    expr: "probe_duration_seconds > 3",
    for: "5m",
    severity: "warning",
    summary: "{{ $labels.instance }} responds slowly",
    description: "Responses take {{ $value | humanize }}s.",
  },
];

export interface JobPreset {
  kind: "exporter" | "blackbox-http" | "blackbox-tcp" | "blackbox-ssl";
  label: string;
  hint: string;
  module: string;
  placeholder: string;
}

export const JOB_PRESETS: JobPreset[] = [
  {
    kind: "exporter",
    label: "Exporter",
    hint: "node_exporter, cAdvisor, or any /metrics endpoint",
    module: "",
    placeholder: "172.22.241.34:9100 # vpn node exporter\n172.22.241.34:8080 # vpn cadvisor",
  },
  {
    kind: "blackbox-http",
    label: "Website check",
    hint: "HTTP answer is 2xx, via the blackbox exporter",
    module: "http_2xx",
    placeholder: "https://example.com\nhttps://shop.example.com",
  },
  {
    kind: "blackbox-tcp",
    label: "Port check",
    hint: "A TCP port accepts connections",
    module: "tcp_connect",
    placeholder: "185.142.159.218:443 # HP server 1",
  },
  {
    kind: "blackbox-ssl",
    label: "SSL certificate",
    hint: "Track certificate expiry, via the blackbox exporter",
    module: "http_2xx",
    placeholder: "https://example.com",
  },
];

// Reads a textarea of targets, one per line, as "address # note". Lines
// pasted from a YAML list ("- address") and trailing words after the
// address are both accepted.
export function parseTargets(text: string): { address: string; comment: string }[] {
  return text
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line && !line.startsWith("#"))
    .map((line) => {
      const hash = line.indexOf("#");
      let address = (hash >= 0 ? line.slice(0, hash) : line).replace(/^-\s*/, "").trim();
      let comment = hash >= 0 ? line.slice(hash + 1).trim() : "";
      const space = address.search(/\s/);
      if (space >= 0) {
        comment = `${address.slice(space).trim()} ${comment}`.trim();
        address = address.slice(0, space);
      }
      return { address: address.replace(/^["']|["']$/g, ""), comment };
    });
}
