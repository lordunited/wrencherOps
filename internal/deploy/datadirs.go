package deploy

import (
	"encoding/base64"
	"fmt"
	"strings"

	"devops-wrapper/internal/sshexec"
)

// DataDir is a host folder that a container bind-mounts and must be able to
// write to. Docker creates missing bind-mount folders as root, but several
// official images run as a fixed non-root user, so without this the
// container fails with "permission denied" on its own data folder.
type DataDir struct {
	Path  string
	Owner string // "uid:gid" the container's process runs as
}

// User IDs the official images run as, per each image's own documentation.
const (
	ownerNexus      = "200:200"     // sonatype/nexus3 runs as "nexus"
	ownerPrometheus = "65534:65534" // prom/prometheus runs as "nobody"
	ownerAlertmgr   = "65534:65534" // prom/alertmanager runs as "nobody"
	ownerGrafana    = "472:0"       // grafana/grafana runs as "grafana", group root
)

// DataDirsFor returns the folders to prepare before starting a service of
// serviceType deployed under deployDir. Paths match the bind mounts the
// generators in this package write. Returns nil for types that need no
// preparation: GitLab's image runs as root and fixes its own permissions,
// and raw docker/kubernetes YAML is the user's own to manage.
func DataDirsFor(serviceType, deployDir string) []DataDir {
	dir := strings.TrimRight(deployDir, "/")
	switch serviceType {
	case "nexus3":
		return []DataDir{{Path: dir + "/nexus-data", Owner: ownerNexus}}
	case "prometheus":
		return []DataDir{
			{Path: dir + "/prometheus-data", Owner: ownerPrometheus},
			{Path: dir + "/grafana-data", Owner: ownerGrafana},
			{Path: dir + "/alertmanager-data", Owner: ownerAlertmgr},
		}
	}
	return nil
}

// SeedFile is a file written at deploy time only if it doesn't exist yet,
// so it provides a starting point without ever overwriting later edits.
type SeedFile struct {
	Path    string
	Content string
}

// SeedFilesFor returns the starting files for a service type. Only the
// Prometheus stack has any: its config and a first alert rule.
func SeedFilesFor(serviceType, deployDir string) []SeedFile {
	if serviceType == "nginx" {
		return NginxSeedFiles(deployDir)
	}
	if serviceType != "prometheus" {
		return nil
	}
	dir := strings.TrimRight(deployDir, "/") + "/" + PrometheusConfigFolder
	return []SeedFile{
		{Path: dir + "/prometheus.yml", Content: DefaultPrometheusConfig},
		{Path: dir + "/rules/availability.yml", Content: DefaultAlertRules},
	}
}

// SeedsMounted reports whether a compose file mounts the folder the seed
// files for serviceType go into. Stacks created before that folder existed
// keep their config elsewhere; seeding them would only leave unused files.
func SeedsMounted(serviceType, manifest string) bool {
	switch serviceType {
	case "prometheus":
		return strings.Contains(manifest, PrometheusConfigFolder+":/etc/prometheus")
	case "nginx":
		return strings.Contains(manifest, NginxConfFolder+":/etc/nginx/conf.d")
	}
	return false
}

// seedFiles writes each file that doesn't exist yet, logging into job.
func seedFiles(job *Job, client *sshexec.Client, out *logWriter, files []SeedFile) bool {
	if len(files) == 0 {
		return true
	}
	job.appendLog("Checking starting config files...\n")
	var b strings.Builder
	b.WriteString(`if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; else AS_ROOT="sudo -n"; fi; set -e`)
	for _, f := range files {
		p := sshexec.ShellQuote(f.Path)
		fmt.Fprintf(&b, `; if [ -e %s ]; then echo "  kept existing "%s; else $AS_ROOT mkdir -p "$(dirname %s)" && echo %s | base64 -d | $AS_ROOT tee %s > /dev/null && echo "  created "%s; fi`,
			p, p, p, base64.StdEncoding.EncodeToString([]byte(f.Content)), p, p)
	}
	if err := client.Run(b.String(), out); err != nil {
		job.appendLog("Could not write starting config files: " + err.Error() + "\n")
		job.setStatus(StatusFailed)
		return false
	}
	return true
}

// prepareDirsCmd builds one shell command that creates each folder and
// hands it to its owner. Ownership is only changed (recursively) when the
// folder's current owner differs, so redeploying over a large existing data
// folder doesn't re-walk it every time. Runs through "sudo -n" when the SSH
// user isn't root; -n makes a missing sudo rule fail fast instead of
// hanging on a password prompt.
func prepareDirsCmd(dirs []DataDir) string {
	var b strings.Builder
	b.WriteString(`if [ "$(id -u)" -eq 0 ]; then AS_ROOT=""; else AS_ROOT="sudo -n"; fi`)
	for _, d := range dirs {
		p := sshexec.ShellQuote(d.Path)
		o := sshexec.ShellQuote(d.Owner)
		fmt.Fprintf(&b, ` && $AS_ROOT mkdir -p %s && { [ "$(stat -c %%u:%%g %s)" = %s ] || $AS_ROOT chown -R %s %s; }`,
			p, p, o, o, p)
	}
	return b.String()
}

// prepareDirs runs prepareDirsCmd on client, logging into job. On failure
// it marks the job failed with a hint and returns false.
func prepareDirs(job *Job, client *sshexec.Client, out *logWriter, dirs []DataDir) bool {
	if len(dirs) == 0 {
		return true
	}
	job.appendLog("Preparing data folders...\n")
	for _, d := range dirs {
		job.appendLog(fmt.Sprintf("  %s (owner %s)\n", d.Path, d.Owner))
	}
	if err := client.Run(prepareDirsCmd(dirs), out); err != nil {
		job.appendLog("Could not prepare data folders: " + err.Error() + "\n" +
			"The SSH user must be root, or be allowed to run sudo without a password.\n")
		job.setStatus(StatusFailed)
		return false
	}
	job.appendLog("Data folders ready.\n")
	return true
}
