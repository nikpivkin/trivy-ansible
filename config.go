package main

import (
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/ini.v1"
)

const (
	ansibleCfgFile    = "ansible.cfg"
	defaultConfigPath = "/etc/ansible/ansible.cfg"
)

type AnsibleConfig struct {
	Inventory []string
	RolesPath []string
}

func readAnsibleConfig(fsys fs.FS, projectPath string) (AnsibleConfig, error) {
	ansibleCfg := AnsibleConfig{}

	cfgpath := resolveAnsibleConfigPath(fsys, projectPath)
	if cfgpath == "" {
		return ansibleCfg, nil
	}

	var source any = cfgpath

	if !filepath.IsAbs(cfgpath) {
		f, err := fsys.Open(cfgpath)
		if err != nil {
			return ansibleCfg, err
		}
		source = f
	}

	cfg, err := ini.Load(source)
	if err != nil {
		return ansibleCfg, err
	}

	ansibleCfg.RolesPath = cfg.Section("defaults").Key("roles_path").Strings(":")

	return ansibleCfg, nil
}

// https://docs.ansible.com/ansible/latest/reference_appendices/config.html#the-configuration-file
func resolveAnsibleConfigPath(fsys fs.FS, projectPath string) string {
	if cfgpath := os.Getenv("ANSIBLE_CONFIG"); cfgpath != "" {
		return cfgpath
	}

	cfgpath := filepath.Join(projectPath, ansibleCfgFile)
	if _, err := fs.Stat(fsys, cfgpath); err == nil {
		return cfgpath
	}

	if homedir, err := os.UserHomeDir(); err == nil {
		cfgpath := filepath.Join(homedir, ansibleCfgFile)
		if _, err := os.Stat(cfgpath); err == nil {
			return cfgpath
		}
	}

	if _, err := os.Stat(defaultConfigPath); err == nil {
		return defaultConfigPath
	}

	return ""
}
