package deploy

import "fmt"

func ExampleGitLabConfig_ComposeYAML() {
	cfg := GitLabConfig{
		ExternalURL:        "http://gitlab.example.com",
		HTTPPort:           80,
		SSHPort:            2222,
		RootPassword:       "testpass123",
		ImageTag:           "latest",
		DeployDir:          "/opt/gitlab",
		ContainerName:      "gitlab",
		ExtraOmnibusConfig: "registry_external_url 'https://registry.example.com'\ngitlab_rails['smtp_enable'] = true\n\ngitlab_rails['smtp_port'] = 587",
		Networks:           []string{"shared_net"},
		ExtraVolumes:       []string{"/opt/gitlab/certs:/etc/gitlab/ssl:ro"},
	}
	fmt.Print(cfg.ComposeYAML())
	// Output:
	// services:
	//   gitlab:
	//     image: gitlab/gitlab-ce:latest
	//     container_name: gitlab
	//     restart: always
	//     hostname: 'gitlab.example.com'
	//     environment:
	//       GITLAB_OMNIBUS_CONFIG: |
	//         external_url 'http://gitlab.example.com'
	//         gitlab_rails['initial_root_password'] = 'testpass123'
	//         gitlab_rails['gitlab_shell_ssh_port'] = 2222
	//         registry_external_url 'https://registry.example.com'
	//         gitlab_rails['smtp_enable'] = true
	//         gitlab_rails['smtp_port'] = 587
	//     ports:
	//       - '80:80'
	//       - '2222:22'
	//     volumes:
	//       - '/opt/gitlab/config:/etc/gitlab'
	//       - '/opt/gitlab/logs:/var/log/gitlab'
	//       - '/opt/gitlab/data:/var/opt/gitlab'
	//       - '/opt/gitlab/certs:/etc/gitlab/ssl:ro'
	//     shm_size: '256m'
	//     healthcheck:
	//       test: ["CMD", "curl", "-f", "http://localhost/-/readiness"]
	//       interval: 30s
	//       timeout: 10s
	//       retries: 5
	//       start_period: 5m
	//     networks:
	//       - shared_net
	//
	// networks:
	//   shared_net:
}

func ExamplePrometheusStackConfig_ComposeYAML() {
	cfg := PrometheusStackConfig{
		PrometheusPort:        9090,
		GrafanaPort:           3000,
		AlertmanagerPort:      9093,
		NodeExporterPort:      9100,
		ImageTag:              "latest",
		GrafanaAdminPassword:  "testpass123",
		RetentionTime:         "15d",
		DeployDir:             "/opt/monitoring",
		ContainerPrefix:       "monitoring",
		Networks:              []string{"shared_net"},
		ExtraVolumes:          []string{"/opt/monitoring/rules:/etc/prometheus/rules:ro"},
		PrometheusCPULimit:    "2",
		PrometheusMemoryLimit: "4g",
	}
	fmt.Print(cfg.ComposeYAML())
	// Output:
	// services:
	//   prometheus:
	//     image: prom/prometheus:latest
	//     container_name: monitoring-prometheus
	//     restart: always
	//     configs:
	//       - source: prometheus_config
	//         target: /etc/prometheus/prometheus.yml
	//     command:
	//       - '--config.file=/etc/prometheus/prometheus.yml'
	//       - '--storage.tsdb.path=/prometheus'
	//       - '--storage.tsdb.retention.time=15d'
	//     ports:
	//       - '9090:9090'
	//     volumes:
	//       - '/opt/monitoring/prometheus-data:/prometheus'
	//       - '/opt/monitoring/rules:/etc/prometheus/rules:ro'
	//     cpus: '2'
	//     mem_limit: 4g
	//     healthcheck:
	//       test: ["CMD", "wget", "--spider", "-q", "http://localhost:9090/-/healthy"]
	//       interval: 30s
	//       timeout: 10s
	//       retries: 5
	//       start_period: 30s
	//     networks:
	//       - shared_net
	//   grafana:
	//     image: grafana/grafana:latest
	//     container_name: monitoring-grafana
	//     restart: always
	//     depends_on:
	//       - prometheus
	//     environment:
	//       - GF_SECURITY_ADMIN_PASSWORD=testpass123
	//     configs:
	//       - source: grafana_datasource
	//         target: /etc/grafana/provisioning/datasources/prometheus.yml
	//     ports:
	//       - '3000:3000'
	//     volumes:
	//       - '/opt/monitoring/grafana-data:/var/lib/grafana'
	//     healthcheck:
	//       test: ["CMD-SHELL", "curl -f http://localhost:3000/api/health || exit 1"]
	//       interval: 30s
	//       timeout: 10s
	//       retries: 5
	//       start_period: 30s
	//     networks:
	//       - shared_net
	//   alertmanager:
	//     image: prom/alertmanager:latest
	//     container_name: monitoring-alertmanager
	//     restart: always
	//     ports:
	//       - '9093:9093'
	//     volumes:
	//       - '/opt/monitoring/alertmanager-data:/alertmanager'
	//     healthcheck:
	//       test: ["CMD", "wget", "--spider", "-q", "http://localhost:9093/-/healthy"]
	//       interval: 30s
	//       timeout: 10s
	//       retries: 5
	//       start_period: 15s
	//     networks:
	//       - shared_net
	//   node-exporter:
	//     image: prom/node-exporter:latest
	//     container_name: monitoring-node-exporter
	//     restart: always
	//     pid: host
	//     ports:
	//       - '9100:9100'
	//     volumes:
	//       - '/proc:/host/proc:ro'
	//       - '/sys:/host/sys:ro'
	//       - '/:/rootfs:ro'
	//     command:
	//       - '--path.procfs=/host/proc'
	//       - '--path.sysfs=/host/sys'
	//       - '--path.rootfs=/rootfs'
	//       - '--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)'
	//     healthcheck:
	//       test: ["CMD", "wget", "--spider", "-q", "http://localhost:9100/"]
	//       interval: 30s
	//       timeout: 10s
	//       retries: 5
	//       start_period: 15s
	//     networks:
	//       - shared_net
	//
	// configs:
	//   prometheus_config:
	//     content: |
	//       global:
	//         scrape_interval: 15s
	//       scrape_configs:
	//         - job_name: 'prometheus'
	//           static_configs:
	//             - targets: ['localhost:9090']
	//         - job_name: 'node-exporter'
	//           static_configs:
	//             - targets: ['node-exporter:9100']
	//         - job_name: 'alertmanager'
	//           static_configs:
	//             - targets: ['alertmanager:9093']
	//       alerting:
	//         alertmanagers:
	//           - static_configs:
	//               - targets: ['alertmanager:9093']
	//   grafana_datasource:
	//     content: |
	//       apiVersion: 1
	//       datasources:
	//         - name: Prometheus
	//           type: prometheus
	//           access: proxy
	//           url: http://prometheus:9090
	//           isDefault: true
	//
	// networks:
	//   shared_net:
}
