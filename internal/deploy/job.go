// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"devops-wrapper/internal/randid"
	"devops-wrapper/internal/sshexec"
)

type JobStatus string

const (
	StatusRunning JobStatus = "running"
	StatusSuccess JobStatus = "success"
	StatusFailed  JobStatus = "failed"
)

// Job tracks one deployment's progress. Output is appended as it streams in
// from the remote command, not necessarily line by line.
type Job struct {
	mu        sync.Mutex
	id        string
	status    JobStatus
	log       []string
	startedAt time.Time
	updatedAt time.Time
}

func (j *Job) appendLog(chunk string) {
	if chunk == "" {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.log = append(j.log, chunk)
	j.updatedAt = time.Now()
}

func (j *Job) setStatus(s JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.status = s
	j.updatedAt = time.Now()
}

// Snapshot is a point-in-time, JSON-friendly view of a Job.
type Snapshot struct {
	ID        string    `json:"id"`
	Status    JobStatus `json:"status"`
	Log       []string  `json:"log"`
	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (j *Job) Snapshot() Snapshot {
	j.mu.Lock()
	defer j.mu.Unlock()
	logCopy := make([]string, len(j.log))
	copy(logCopy, j.log)
	return Snapshot{ID: j.id, Status: j.status, Log: logCopy, StartedAt: j.startedAt, UpdatedAt: j.updatedAt}
}

// logWriter adapts a Job into an io.Writer so ssh command output can stream
// straight into its log as it arrives.
type logWriter struct {
	job *Job
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.job.appendLog(string(p))
	return len(p), nil
}

// jobRetention is how long a finished job's log stays fetchable before the
// Store forgets it. Without this, Store.jobs grows by one entry — plus
// whatever output it collected — on every single Deploy/Start/Stop/Down,
// forever, for as long as the process runs. A completed job's log is
// already understood to be best-effort (it doesn't survive a restart
// either), so this doesn't change what callers can rely on, just how long
// a finished job stays around.
const jobRetention = time.Hour

// Store holds in-flight and completed jobs in memory. Like the rest of this
// backend, it doesn't survive a restart — acceptable for a single-instance
// tool with no database.
type Store struct {
	mu   sync.Mutex
	jobs map[string]*Job
}

func NewStore() *Store {
	return &Store{jobs: make(map[string]*Job)}
}

func (s *Store) create() *Job {
	job := &Job{id: randomID(), status: StatusRunning, log: []string{}, startedAt: time.Now(), updatedAt: time.Now()}
	s.mu.Lock()
	s.jobs[job.id] = job
	s.sweep()
	s.mu.Unlock()
	return job
}

// sweep evicts finished jobs last updated more than jobRetention ago.
// Called with mu already held; runs on every create() rather than on its
// own ticker, since that's already exactly as often as this needs to run
// and doesn't need a second background goroutine to do it.
func (s *Store) sweep() {
	cutoff := time.Now().Add(-jobRetention)
	for id, j := range s.jobs {
		j.mu.Lock()
		stale := j.status != StatusRunning && j.updatedAt.Before(cutoff)
		j.mu.Unlock()
		if stale {
			delete(s.jobs, id)
		}
	}
}

func (s *Store) Get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	return j, ok
}

// applySpec describes how to bring up one kind of YAML file (a compose file,
// a Kubernetes manifest, ...) once it's on the target: what to name it, how
// to confirm the real tool we're about to run actually works, and the
// command that applies it.
type applySpec struct {
	filename string

	// checkCmd must exit 0 before applyCmd runs. This checks the actual
	// command we're about to use — e.g. "docker compose version", not
	// just "command -v docker" — so a missing compose plugin fails here
	// with a clear message instead of surfacing as a confusing error
	// from whatever applyCmd happens to fall back to.
	checkCmd    string
	checkDesc   string // human-readable, for the log line
	installHint string

	applyCmd func(deployDir, filePath string) string
	verb     string // e.g. "Starting services", "Applying manifest" — for the log line
	doneMsg  string

	// dataDirs are created and handed to their container's user before
	// applyCmd runs (see DataDirsFor).
	dataDirs []DataDir

	// files are the project's own files (.env, configs, ...), written into
	// the deploy folder before applyCmd runs, always overwriting; see
	// writeProjectFiles.
	files      []ProjectFile
	trackFiles bool

	// seeds are written before applyCmd runs, only where missing.
	seeds []SeedFile
}

// dial connects to target, logging progress into job. On failure it marks
// the job failed and returns nil — callers should return immediately when
// that happens.
func dial(job *Job, target sshexec.Target) *sshexec.Client {
	job.appendLog(fmt.Sprintf("Connecting to %s@%s:%d...\n", target.Username, target.Host, target.Port))
	client, err := sshexec.Dial(target)
	if err != nil {
		job.appendLog("SSH connection failed: " + err.Error() + "\n")
		job.setStatus(StatusFailed)
		return nil
	}
	job.appendLog("Connected.\n")
	return client
}

// checkAvailable runs checkCmd on client to confirm the tool an upcoming
// command depends on actually works, logging checkDesc. On failure it marks
// the job failed with installHint and returns false.
func checkAvailable(job *Job, client *sshexec.Client, out *logWriter, checkCmd, checkDesc, installHint string) bool {
	job.appendLog(fmt.Sprintf("Checking %s...\n", checkDesc))
	if err := client.Run(checkCmd, out); err != nil {
		job.appendLog(fmt.Sprintf("%s is not available on the target server. %s\n", checkDesc, installHint))
		job.setStatus(StatusFailed)
		return false
	}
	return true
}

// run writes content to deployDir/spec.filename on target over SSH and
// applies it per spec, in the background. Returns immediately with a Job
// the caller can poll. Shared by every deployable YAML kind.
func (s *Store) run(target sshexec.Target, content, deployDir string, spec applySpec) *Job {
	job := s.create()
	filePath := deployDir + "/" + spec.filename

	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.appendLog(fmt.Sprintf("panic: %v\n", r))
				job.setStatus(StatusFailed)
			}
		}()

		client := dial(job, target)
		if client == nil {
			return
		}
		defer client.Close()

		out := &logWriter{job: job}

		job.appendLog("Writing " + filePath + "...\n")
		if err := client.WriteFile(filePath, []byte(content), out); err != nil {
			job.appendLog("Failed to write file: " + err.Error() + "\n")
			job.setStatus(StatusFailed)
			return
		}
		job.appendLog("File written.\n")
		if spec.trackFiles && !writeProjectFiles(job, client, out, deployDir, spec.files) {
			return
		}

		if !checkAvailable(job, client, out, spec.checkCmd, spec.checkDesc, spec.installHint) {
			return
		}
		if !prepareDirs(job, client, out, spec.dataDirs) {
			return
		}
		if !seedFiles(job, client, out, spec.seeds) {
			return
		}

		job.appendLog(spec.verb + "...\n")
		if err := client.Run(spec.applyCmd(deployDir, filePath), out); err != nil {
			job.appendLog("Apply failed: " + err.Error() + "\n")
			job.setStatus(StatusFailed)
			return
		}

		job.appendLog(spec.doneMsg + "\n")
		job.setStatus(StatusSuccess)
	}()

	return job
}

// commandSpec describes a single control command run against an
// already-deployed project — no file write, unlike applySpec.
type commandSpec struct {
	checkCmd    string
	checkDesc   string
	installHint string
	cmd         string
	verb        string
	doneMsg     string
	dataDirs    []DataDir // prepared before cmd runs, as in applySpec
}

// runCommand connects to target and runs spec.cmd, in the background.
// Shared by control actions (stop, down) that act on a project already on
// disk rather than deploying new YAML.
func (s *Store) runCommand(target sshexec.Target, spec commandSpec) *Job {
	job := s.create()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				job.appendLog(fmt.Sprintf("panic: %v\n", r))
				job.setStatus(StatusFailed)
			}
		}()

		client := dial(job, target)
		if client == nil {
			return
		}
		defer client.Close()

		out := &logWriter{job: job}

		if !checkAvailable(job, client, out, spec.checkCmd, spec.checkDesc, spec.installHint) {
			return
		}
		if !prepareDirs(job, client, out, spec.dataDirs) {
			return
		}

		job.appendLog(spec.verb + "...\n")
		if err := client.Run(spec.cmd, out); err != nil {
			job.appendLog("Command failed: " + err.Error() + "\n")
			job.setStatus(StatusFailed)
			return
		}

		job.appendLog(spec.doneMsg + "\n")
		job.setStatus(StatusSuccess)
	}()

	return job
}

// RunCompose writes composeYAML to deployDir and brings it up with
// docker compose (the "docker compose" CLI plugin — not the standalone,
// hyphenated "docker-compose" binary, which is deprecated upstream and
// often simply absent on a fresh host).
func (s *Store) RunCompose(target sshexec.Target, composeYAML, deployDir string, dataDirs []DataDir, seeds []SeedFile) *Job {
	return s.run(target, composeYAML, deployDir, applySpec{
		dataDirs:    dataDirs,
		seeds:       seeds,
		filename:    "docker-compose.yml",
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        "Starting services (docker compose up -d)... this can pull multi-GB images on first run and take several minutes",
		doneMsg:     "Containers started.",
		applyCmd: func(deployDir, _ string) string {
			return sshexec.AsRootPrelude + sshexec.ComposeIn(deployDir) + " up -d"
		},
	})
}

// RunComposeProject deploys a custom compose project: its compose file plus
// every file of the project (.env, configs, scripts, ...) into deployDir,
// then "docker compose up -d --remove-orphans", so a service deleted from
// the compose has its container removed too (its volumes stay).
func (s *Store) RunComposeProject(target sshexec.Target, composeYAML, deployDir, composeFile string, files []ProjectFile) *Job {
	if composeFile == "" {
		composeFile = "docker-compose.yml"
	}
	return s.run(target, composeYAML, deployDir, applySpec{
		files:       files,
		trackFiles:  true,
		filename:    composeFile,
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        "Starting services (docker compose up -d --remove-orphans)... this can pull images on first run and take a while",
		doneMsg:     "Containers started.",
		applyCmd: func(deployDir, _ string) string {
			return sshexec.AsRootPrelude + sshexec.ComposeIn(deployDir) + " up -d --remove-orphans"
		},
	})
}

// Project addresses a Compose project on the server. A stack the panel
// deployed is just its folder, using the default compose file there. A
// connected stack also carries its project name and compose files, which
// can have any name and live anywhere.
type Project struct {
	Dir   string
	Name  string
	Files []string
}

// Command is the "docker compose" invocation for the project, ready for a
// subcommand to be appended. Callers must prepend sshexec.AsRootPrelude —
// this only inserts the "$AS_ROOT" it defines, right before "docker" so
// "cd"/other shell logic around it never runs through sudo.
func (p Project) Command() string {
	if p.Name == "" && len(p.Files) == 0 {
		return sshexec.ComposeIn(p.Dir)
	}
	cmd := "$AS_ROOT docker compose"
	if p.Name != "" {
		cmd += " -p " + sshexec.ShellQuote(p.Name)
	}
	if p.Dir != "" {
		cmd += " --project-directory " + sshexec.ShellQuote(p.Dir)
	}
	for _, f := range p.Files {
		cmd += " -f " + sshexec.ShellQuote(f)
	}
	return cmd
}

// RunComposeStart brings a project back up with "docker compose up -d"
// against whatever docker-compose.yml is already in deployDir — unlike
// RunCompose, it doesn't rewrite the file first. This works whether the
// project was stopped with RunComposeStop (containers still exist, this
// just starts them) or taken all the way down with RunComposeDown
// (containers were removed, this recreates them) — "up -d" handles both.
func (s *Store) RunComposeStart(target sshexec.Target, project Project, dataDirs []DataDir) *Job {
	return s.runCommand(target, commandSpec{
		dataDirs:    dataDirs,
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        "Starting services (docker compose up -d)",
		doneMsg:     "Containers started.",
		cmd:         sshexec.AsRootPrelude + project.Command() + " up -d",
	})
}

// RunComposeStop stops (but does not remove) the containers for the
// compose project in deployDir — "docker compose stop". Containers,
// networks, and volumes are left intact, so a later Deploy or a manual
// "docker compose start" picks up right where it left off.
func (s *Store) RunComposeStop(target sshexec.Target, project Project) *Job {
	return s.runCommand(target, commandSpec{
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        "Stopping services (docker compose stop)",
		doneMsg:     "Containers stopped.",
		cmd:         sshexec.AsRootPrelude + project.Command() + " stop",
	})
}

// RunComposeDown removes the containers and default network for the
// compose project in deployDir — "docker compose down". Named volumes are
// left alone (this never passes -v), so persisted data survives.
func (s *Store) RunComposeDown(target sshexec.Target, project Project) *Job {
	return s.runCommand(target, commandSpec{
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        "Removing containers (docker compose down)",
		doneMsg:     "Containers removed.",
		cmd:         sshexec.AsRootPrelude + project.Command() + " down",
	})
}

// RunComposeDestroy removes the containers, network, and volumes for the
// compose project in deployDir ("docker compose down -v"), then deletes
// deployDir itself — the compose file and everything under it, including
// any bind-mounted data. This is irreversible; callers must have the user
// confirm before calling it. "docker compose down -v" against a project
// that's already stopped/removed is a safe no-op (verified), so this is
// also safe to call on a service that was never fully deployed.
func (s *Store) RunComposeDestroy(target sshexec.Target, composeDir, deployDir string) *Job {
	return s.runCommand(target, commandSpec{
		checkCmd:    sshexec.AsRoot("docker compose version"),
		checkDesc:   "Docker Compose is available",
		installHint: "Install Docker with the Compose plugin: https://docs.docker.com/compose/install/",
		verb:        fmt.Sprintf("Removing containers, volumes, and %s", deployDir),
		doneMsg:     fmt.Sprintf("Containers, volumes, and %s removed.", deployDir),
		cmd:         sshexec.AsRootPrelude + sshexec.ComposeIn(composeDir) + " down -v && $AS_ROOT rm -rf " + sshexec.ShellQuote(deployDir),
	})
}

// RunKubernetesApply writes manifestYAML to deployDir and applies it with
// kubectl, using whatever cluster access is already configured for the SSH
// user on the target (its default kubeconfig) — this tool doesn't manage
// kubeconfigs itself. Deliberately not run via sudo, unlike the Docker
// commands elsewhere in this file: kubectl's access comes from the
// connecting user's own kubeconfig (usually ~/.kube/config), and sudo
// would switch to root's, most likely breaking cluster access rather than
// fixing a permissions problem.
func (s *Store) RunKubernetesApply(target sshexec.Target, manifestYAML, deployDir string) *Job {
	return s.run(target, manifestYAML, deployDir, applySpec{
		filename:    "manifest.yaml",
		checkCmd:    "command -v kubectl",
		checkDesc:   "kubectl is available",
		installHint: "Install it first: https://kubernetes.io/docs/tasks/tools/",
		verb:        "Applying manifest (kubectl apply)",
		doneMsg:     "Manifest applied.",
		applyCmd: func(_, filePath string) string {
			return "kubectl apply -f " + sshexec.ShellQuote(filePath)
		},
	})
}

func randomID() string {
	id, err := randid.New(16)
	if err != nil {
		// crypto/rand failing is effectively unrecoverable; a predictable
		// fallback here would only matter if urandom is broken, at which
		// point the process has bigger problems than a job ID collision.
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
	return id
}
