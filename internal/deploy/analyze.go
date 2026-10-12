// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package deploy

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Mount is one volume a service of a compose project uses, and where its
// data actually lives on the server.
type Mount struct {
	Service   string `json:"service"`
	Kind      string `json:"kind"`             // "bind", "volume", "anonymous" or "tmpfs"
	Source    string `json:"source,omitempty"` // as written, after ${VAR} substitution
	Target    string `json:"target"`
	HostPath  string `json:"hostPath,omitempty"`  // bind: the path on the server
	Volume    string `json:"volume,omitempty"`    // volume: the Docker volume's real name
	ReadOnly  bool   `json:"readOnly,omitempty"`  //
	InProject bool   `json:"inProject,omitempty"` // bind: provided by the project's own files
	External  bool   `json:"external,omitempty"`  // volume: declared external, not created by compose
}

// Analysis is what a compose project's volumes and variables come to on a
// server, and what in it is likely to go wrong.
type Analysis struct {
	Project    string   `json:"project"` // the Compose project name its containers and volumes get
	Mounts     []Mount  `json:"mounts"`
	EnvFiles   []string `json:"envFiles,omitempty"` // env_file paths the services read, as written
	Warnings   []string `json:"warnings"`
	ParseError string   `json:"parseError,omitempty"`
}

// fileLike are extensions whose bind-mount target is almost certainly a
// file. When the source isn't in the project, Docker creates a folder at
// the source instead, mounts that, and the container fails to read it.
var fileLike = regexp.MustCompile(`\.(conf|cfg|ini|toml|ya?ml|json|xml|properties|env|sql|sh|js|py|rb|php|lua|txt|pem|crt|key|cert|html?)$`)

// AnalyzeProject reads a compose project the way "docker compose" does —
// ${VAR}s filled in from .env first, then the YAML — and describes each
// volume and env_file, with warnings for what's likely to break.
func AnalyzeProject(composeYAML, env string, files []ProjectFile, deployDir string) Analysis {
	a := Analysis{Mounts: []Mount{}, Warnings: []string{}}
	vars := parseEnv(env)
	text, missing := interpolate(composeYAML, vars)
	for _, name := range missing {
		a.Warnings = append(a.Warnings, fmt.Sprintf("The compose uses ${%s}, which .env doesn't set, so it becomes empty.", name))
	}

	var doc struct {
		Name     string `yaml:"name"`
		Services map[string]struct {
			Volumes []yaml.Node `yaml:"volumes"`
			EnvFile yaml.Node   `yaml:"env_file"`
		} `yaml:"services"`
		Volumes map[string]*struct {
			Name     string `yaml:"name"`
			External bool   `yaml:"external"`
		} `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil {
		a.ParseError = err.Error()
		return a
	}

	// Compose's own precedence: COMPOSE_PROJECT_NAME (.env counts), then the
	// top-level name, then the folder's name. Verified against real compose:
	// only a name taken from the folder is cleaned up ("My App" -> "myapp");
	// an explicit one that isn't valid makes compose refuse to start.
	explicit, from := vars["COMPOSE_PROJECT_NAME"], "COMPOSE_PROJECT_NAME in .env"
	if explicit == "" {
		explicit, from = doc.Name, "the compose's top-level name:"
	}
	if explicit != "" {
		a.Project = explicit
		if !validProjectNameRe.MatchString(explicit) {
			a.Warnings = append(a.Warnings, fmt.Sprintf("The project name %q (from %s) isn't valid, so compose will refuse to start. Use only lowercase letters, digits, - and _, starting with a letter or digit.", explicit, from))
		}
	} else {
		a.Project = normalizeProjectName(path.Base(deployDir))
	}

	inProject := map[string]bool{}
	for _, f := range files {
		inProject[f.Path] = true
	}
	if env != "" {
		inProject[".env"] = true
	}
	// provided reports whether a path relative to the deploy folder is a
	// project file, or a folder holding some.
	provided := func(rel string) bool {
		if rel == "." || inProject[rel] {
			return true
		}
		for p := range inProject {
			if strings.HasPrefix(p, rel+"/") {
				return true
			}
		}
		return false
	}

	services := make([]string, 0, len(doc.Services))
	for name := range doc.Services {
		services = append(services, name)
	}
	sort.Strings(services)
	for _, svc := range services {
		s := doc.Services[svc]
		for _, node := range s.Volumes {
			m, ok := readMount(svc, &node)
			if !ok {
				continue
			}
			switch m.Kind {
			case "bind":
				switch {
				case strings.HasPrefix(m.Source, "/"):
					m.HostPath = path.Clean(m.Source)
					a.Warnings = append(a.Warnings, fmt.Sprintf("%s mounts %s, outside the deploy folder: it must already exist on every server, and Delete won't remove it.", svc, m.HostPath))
				case strings.HasPrefix(m.Source, "~"):
					m.HostPath = m.Source + " (the SSH user's home)"
				default:
					rel := path.Clean(m.Source)
					m.HostPath = path.Join(deployDir, rel)
					if rel == ".." || strings.HasPrefix(rel, "../") {
						a.Warnings = append(a.Warnings, fmt.Sprintf("%s mounts %s, outside the deploy folder: Delete won't remove it.", svc, m.HostPath))
						break
					}
					m.InProject = provided(rel)
					if !m.InProject && fileLike.MatchString(m.Target) {
						a.Warnings = append(a.Warnings, fmt.Sprintf("%s mounts %s at %s, but the project has no file %s — Docker will create an empty folder there instead, and the container will most likely fail. Add the file to the project.", svc, m.Source, m.Target, rel))
					}
				}
			case "volume":
				def, declared := doc.Volumes[m.Source]
				switch {
				case !declared:
					a.Warnings = append(a.Warnings, fmt.Sprintf("%s uses the volume %q, which isn't declared under the top-level volumes: — compose will refuse to start.", svc, m.Source))
				case def != nil && def.External:
					m.External = true
					m.Volume = m.Source
					if def.Name != "" {
						m.Volume = def.Name
					}
				case def != nil && def.Name != "":
					m.Volume = def.Name
				default:
					m.Volume = a.Project + "_" + m.Source
				}
			}
			a.Mounts = append(a.Mounts, m)
		}
		for _, ef := range envFiles(&s.EnvFile) {
			a.EnvFiles = append(a.EnvFiles, ef)
			rel := path.Clean(ef)
			if !strings.HasPrefix(ef, "/") && !inProject[rel] {
				a.Warnings = append(a.Warnings, fmt.Sprintf("%s reads env_file %s, which isn't in the project — compose will refuse to start unless it's on the server already.", svc, ef))
			}
		}
	}
	return a
}

// readMount reads one entry of a service's volumes:, short syntax
// ("src:dst[:mode]", or just "dst" for an anonymous volume) or long.
func readMount(svc string, n *yaml.Node) (Mount, bool) {
	m := Mount{Service: svc}
	switch n.Kind {
	case yaml.ScalarNode:
		parts := strings.Split(n.Value, ":")
		switch len(parts) {
		case 1:
			m.Kind, m.Target = "anonymous", parts[0]
			return m, true
		default:
			m.Source, m.Target = parts[0], parts[1]
			if len(parts) > 2 {
				m.ReadOnly = strings.Contains(","+parts[2]+",", ",ro,")
			}
		}
	case yaml.MappingNode:
		var long struct {
			Type     string `yaml:"type"`
			Source   string `yaml:"source"`
			Target   string `yaml:"target"`
			ReadOnly bool   `yaml:"read_only"`
		}
		if n.Decode(&long) != nil {
			return m, false
		}
		m.Source, m.Target, m.ReadOnly = long.Source, long.Target, long.ReadOnly
		switch long.Type {
		case "tmpfs":
			m.Kind = "tmpfs"
			return m, true
		case "volume":
			if m.Source == "" {
				m.Kind = "anonymous"
				return m, true
			}
			m.Kind = "volume"
			return m, true
		case "bind":
			m.Kind = "bind"
			return m, true
		}
	default:
		return m, false
	}
	if strings.HasPrefix(m.Source, "/") || strings.HasPrefix(m.Source, ".") || strings.HasPrefix(m.Source, "~") {
		m.Kind = "bind"
	} else {
		m.Kind = "volume"
	}
	return m, true
}

// envFiles reads a service's env_file: one path, a list of paths, or a list
// of {path, required}.
func envFiles(n *yaml.Node) []string {
	switch n.Kind {
	case yaml.ScalarNode:
		return []string{n.Value}
	case yaml.SequenceNode:
		var out []string
		for _, item := range n.Content {
			switch item.Kind {
			case yaml.ScalarNode:
				out = append(out, item.Value)
			case yaml.MappingNode:
				var e struct {
					Path     string `yaml:"path"`
					Required *bool  `yaml:"required"`
				}
				if item.Decode(&e) == nil && e.Path != "" && (e.Required == nil || *e.Required) {
					out = append(out, e.Path)
				}
			}
		}
		return out
	}
	return nil
}

// parseEnv reads a .env file the way compose does for interpolation:
// KEY=VALUE lines, "#" comments, an optional "export ", and surrounding
// quotes stripped.
func parseEnv(env string) map[string]string {
	vars := map[string]string{}
	for _, line := range strings.Split(env, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
			v = v[1 : len(v)-1]
		}
		vars[k] = v
	}
	return vars
}

var varRe = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?+])([^}]*))?\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// interpolate substitutes ${VAR}, ${VAR:-default}, ${VAR-default},
// ${VAR:+alt}, ${VAR:?err} and $VAR from vars, with "$$" as a literal "$",
// and returns the names used without a value or a default.
func interpolate(s string, vars map[string]string) (string, []string) {
	const escaped = "\x00"
	s = strings.ReplaceAll(s, "$$", escaped)
	seen := map[string]bool{}
	var missing []string
	out := varRe.ReplaceAllStringFunc(s, func(m string) string {
		g := varRe.FindStringSubmatch(m)
		name, op, arg := g[1], g[2], g[3]
		if name == "" {
			name = g[4]
		}
		val, set := vars[name]
		switch op {
		case ":-":
			if val == "" {
				return arg
			}
			return val
		case "-":
			if !set {
				return arg
			}
			return val
		case ":+":
			if val != "" {
				return arg
			}
			return ""
		case "+":
			if set {
				return arg
			}
			return ""
		}
		if !set && !seen[name] {
			seen[name] = true
			missing = append(missing, name)
		}
		return val
	})
	return strings.ReplaceAll(out, escaped, "$"), missing
}

var (
	projectNameRe      = regexp.MustCompile(`[^a-z0-9_-]`)
	validProjectNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
)

// normalizeProjectName turns a folder name into a project name the way
// compose does: lower case, only a-z 0-9 _ -, starting with a letter or
// digit.
func normalizeProjectName(s string) string {
	s = projectNameRe.ReplaceAllString(strings.ToLower(s), "")
	return strings.TrimLeft(s, "_-")
}

// DefaultProjectName is the project name compose gives a project in dir
// when nothing else names it: the folder's name, cleaned up.
func DefaultProjectName(dir string) string {
	return normalizeProjectName(path.Base(dir))
}
