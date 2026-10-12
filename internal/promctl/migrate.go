// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package promctl

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigFolder is where file-based Prometheus configuration lives, relative
// to a service's deploy directory. It's mounted at /etc/prometheus.
const ConfigFolder = "prometheus-config"

// MoveConfigToDisk rewrites a docker-compose file whose Prometheus reads an
// inline Compose config (the layout older panel-generated stacks used) so
// it reads deployDir/prometheus-config instead, bind-mounted at
// /etc/prometheus. Inline configs are copied into the container when it's
// created, so any edit made there is lost on the next redeploy; a bind
// mount makes edits durable.
func MoveConfigToDisk(manifest, deployDir string) (string, error) {
	doc, root, err := parseDoc(manifest)
	if err != nil {
		return "", err
	}
	services := mapGet(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return "", fmt.Errorf("the compose file has no services")
	}

	var prom *yaml.Node
	for i := 0; i+1 < len(services.Content); i += 2 {
		svc := services.Content[i+1]
		if img := mapGet(svc, "image"); img != nil && isPrometheusImage(img.Value) {
			prom = svc
			break
		}
	}
	if prom == nil {
		return "", fmt.Errorf("the compose file has no Prometheus service")
	}

	// Drop the inline config mounted at /etc/prometheus/*, keeping any others.
	var removed []string
	if configs := mapGet(prom, "configs"); configs != nil && configs.Kind == yaml.SequenceNode {
		kept := configs.Content[:0]
		for _, item := range configs.Content {
			source, target := item.Value, ""
			if item.Kind == yaml.MappingNode {
				if n := mapGet(item, "source"); n != nil {
					source = n.Value
				}
				if n := mapGet(item, "target"); n != nil {
					target = n.Value
				}
			}
			if strings.HasPrefix(target, "/etc/prometheus/") || source == "prometheus_config" {
				removed = append(removed, source)
				continue
			}
			kept = append(kept, item)
		}
		configs.Content = kept
		if len(kept) == 0 {
			removeKey(prom, "configs")
		}
	}

	volume := strings.TrimRight(deployDir, "/") + "/" + ConfigFolder + ":/etc/prometheus"
	volumes := mapGet(prom, "volumes")
	if volumes == nil || volumes.Kind != yaml.SequenceNode {
		volumes = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		mapSet(prom, "volumes", volumes)
	}
	present := false
	for _, v := range volumes.Content {
		if v.Value == volume {
			present = true
		}
	}
	if !present {
		n := scalar(volume)
		n.Style = yaml.SingleQuotedStyle
		volumes.Content = append(volumes.Content, n)
	}

	// Remove the now-unused top-level config definitions.
	if top := mapGet(root, "configs"); top != nil && top.Kind == yaml.MappingNode {
		for _, name := range removed {
			if stillUsed(services, name) {
				continue
			}
			removeKey(top, name)
		}
		if len(top.Content) == 0 {
			removeKey(root, "configs")
		}
	}
	return encodeDoc(doc)
}

func removeKey(m *yaml.Node, key string) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return
		}
	}
}

func stillUsed(services *yaml.Node, configName string) bool {
	for i := 0; i+1 < len(services.Content); i += 2 {
		configs := mapGet(services.Content[i+1], "configs")
		if configs == nil {
			continue
		}
		for _, item := range configs.Content {
			if item.Value == configName {
				return true
			}
			if n := mapGet(item, "source"); n != nil && n.Value == configName {
				return true
			}
		}
	}
	return false
}
