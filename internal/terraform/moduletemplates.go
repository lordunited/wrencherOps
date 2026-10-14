// Copyright (c) 2026 Mohammadreza Masoumi <s.masoumimohammadreza@gmail.com>
// Author: Mohammadreza Masoumi, DevOps Engineer

package terraform

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// moduleTemplateFS holds the module workspaces: one folder each, a root
// module that calls a reusable module under modules/<name>/. They're real
// .tf files, so they're formatted and validated as code.
//
//go:embed moduletemplates
var moduleTemplateFS embed.FS

// moduleTemplates lists the folders of moduleTemplateFS, in the order the
// panel offers them.
var moduleTemplates = []struct{ Key, Name, Description string }{
	{"esxi-vm", "ESXi: virtual machine", "A VM on a standalone ESXi host, from a template VM or empty. Needs SSH enabled on the host."},
	{"vsphere-vm", "vCenter: virtual machine", "A VM cloned from a vSphere template, with CPU, memory, disk, network and guest customization."},
	{"virtualbox-vm", "VirtualBox: virtual machine", "VMs from a Vagrant box image. Runs VBoxManage where OpenTofu runs, so plan it from a machine with VirtualBox, not the panel."},
	{"proxmox-vm", "Proxmox VE: virtual machine", "A VM cloned from a Proxmox template, with cloud-init user, SSH key and IP."},
	{"vault-app", "Vault: app secrets and AppRole", "A KV v2 mount with an app's secrets, a read-only policy, and an AppRole the app logs in with."},
	{"docker-container", "Docker: container", "An image pulled and run as a container on a Docker host (local socket or ssh://), with ports, env and volumes."},
	{"kubernetes-app", "Kubernetes: app", "A namespace, Deployment and Service for one container image, with probes and resource limits."},
	{"helm-release", "Helm: chart release", "A chart from a Helm repository installed into a namespace, with values in YAML."},
	{"aws-ec2", "AWS: EC2 instance", "An EC2 instance with its own security group, in the default VPC unless you pick a subnet."},
	{"azure-vm", "Azure: Linux VM", "A resource group, network, public IP and Ubuntu VM with SSH key login."},
	{"gcp-vm", "Google Cloud: VM", "A Compute Engine instance with a firewall rule for the ports you open."},
	{"cloudflare-dns", "Cloudflare: DNS records", "DNS records in an existing Cloudflare zone, from a map."},
	{"gitlab-project", "GitLab: project", "A GitLab project in a group, with protected branch, CI/CD variables and deploy token."},
}

// loadModuleTemplates reads every module workspace from moduleTemplateFS.
func loadModuleTemplates() ([]Template, error) {
	out := make([]Template, 0, len(moduleTemplates))
	for _, m := range moduleTemplates {
		dir := path.Join("moduletemplates", m.Key)
		var files []File
		err := fs.WalkDir(moduleTemplateFS, dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			b, err := moduleTemplateFS.ReadFile(p)
			if err != nil {
				return err
			}
			files = append(files, File{Path: strings.TrimPrefix(p, dir+"/"), Content: string(b)})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", m.Key, err)
		}
		out = append(out, Template{Key: m.Key, Name: m.Name, Description: m.Description, Files: files})
	}
	return out, nil
}

func init() {
	ts, err := loadModuleTemplates()
	if err != nil {
		panic(err) // the folders are embedded at build time: only a broken build gets here
	}
	Templates = append(Templates, ts...)
}
