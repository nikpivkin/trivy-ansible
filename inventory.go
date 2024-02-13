package main

import (
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/samber/lo"
	"gopkg.in/ini.v1"
	"gopkg.in/yaml.v3"
)

const defaultInventoryPath = "/etc/ansible/hosts"

type HostGroup struct {
	hosts    []string
	children []string
}

type Inventory struct {
	groups   map[string]*HostGroup
	hostVars map[string]map[string]string
}

func NewInventory() Inventory {
	return Inventory{
		groups:   make(map[string]*HostGroup),
		hostVars: make(map[string]map[string]string),
	}
}

func (i *Inventory) AddHosts(groupName string, hosts []string) {
	group, exists := i.groups[groupName]
	if !exists {
		group = &HostGroup{
			hosts: hosts,
		}
		i.groups[groupName] = group
	} else {
		group.hosts = append(group.hosts, hosts...)
	}
}
func (i *Inventory) AddChildren(groupName string, children []string) {
	group, exists := i.groups[groupName]
	if !exists {
		group = &HostGroup{
			children: children,
		}
		i.groups[groupName] = group
	} else {
		group.children = append(group.children, children...)
	}
}
func (i *Inventory) AddGroupVars(groupName string, vars map[string]string) {
	if groupName == "all" {
		// TODO add vars to all hosts
	}
	group, exists := i.groups[groupName]
	if !exists {
		log.Printf("group %q does not exists", groupName)
		return
	}
	for _, host := range group.hosts {
		i.AddHostVars(host, vars)
	}

	for _, childGroup := range group.children {
		i.AddGroupVars(childGroup, vars)
	}
}

func (i *Inventory) AddHostVars(host string, vars map[string]string) {
	i.hostVars[host] = lo.Assign(i.hostVars[host], vars)
}

func parseInventories(fsys fs.FS, project *AnsibleProject) {
	paths := []string{defaultInventoryPath, "inventory"}
	paths = append(paths, project.cfg.Inventory...)

	for _, inventoryPath := range paths {
		entries, _ := fs.ReadDir(fsys, inventoryPath) // TODO handle error
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}

			f, err := fsys.Open(filepath.Join(inventoryPath, entry.Name()))
			if err != nil {
				return
			}
			defer f.Close() // TODO

			switch filepath.Ext(entry.Name()) {
			case "yml", "yaml":
				_ = parseYAMLInventory(f)
			case "": // TODO ini?
				_ = parseINIInventory(f)
			}
			// TODO merge inventories
		}

	}

}

func parseYAMLInventory(r io.Reader) Inventory {
	inventory := NewInventory()
	return inventory
}

func parseINIInventory(r io.Reader) Inventory {
	inventory := NewInventory()

	conf, err := ini.LoadSources(ini.LoadOptions{
		AllowBooleanKeys:   true,
		KeyValueDelimiters: " ",
	}, r)
	if err != nil {
		return inventory
	}

	vars := make(map[string]*ini.Section)

	// TODO resolve all group

	for _, section := range conf.Sections() {
		if groupName, ok := strings.CutSuffix(section.Name(), ":vars"); ok {
			vars[groupName] = section
		} else if groupName, ok := strings.CutSuffix(section.Name(), ":children"); ok {
			var children []string
			for _, key := range section.Keys() {
				children = append(children, key.Name())
			}
			inventory.AddChildren(groupName, children)
		} else {
			groupName := section.Name()
			if groupName == "DEFAULT" {
				groupName = "ungrouped"
			}
			var hosts []string
			for _, key := range section.Keys() {
				host := key.Name()
				// TODO handle quotes
				// https://docs.ansible.com/ansible/latest/inventory_guide/intro_inventory.html#defining-variables-in-ini-format
				variables := strings.Split(key.Value(), " ")
				hostVars := make(map[string]string)
				for _, variable := range variables {
					parts := strings.SplitN(variable, "=", 2)
					if len(parts) != 2 {
						continue
					}
					hostVars[parts[0]] = parts[1]
				}
				inventory.AddHostVars(host, hostVars)

				hosts = append(hosts, host)
			}
			inventory.AddHosts(groupName, hosts)
		}
	}

	for groupName, secrion := range vars {
		inventory.AddGroupVars(groupName, secrion.KeysHash())
	}

	return inventory
}

func parseGroupVars(fsys fs.FS, path string) (map[string]map[string]string, error) {
	return parseInventoryVars(fsys, filepath.Join(path, "group_vars"))
}

func parseHostsVars(fsys fs.FS, path string) (map[string]map[string]string, error) {
	return parseInventoryVars(fsys, filepath.Join(path, "host_vars"))
}

func parseInventoryVars(fsys fs.FS, path string) (map[string]map[string]string, error) {
	vars := make(map[string]map[string]string)
	walkFn := func(path string, d fs.DirEntry) error {
		if d.IsDir() {
			return nil
		}
		parts := strings.Split(path, string(os.PathSeparator))
		if len(parts) < 2 {
			return nil
		}
		// host or group
		name := parts[1]

		ext := filepath.Ext(path)
		if !slices.Contains([]string{"", "yml", "yaml", "json"}, ext) {
			return nil
		}
		f, err := fsys.Open(path)
		if err != nil {
			return err
		}

		var v map[string]string
		if err := yaml.NewDecoder(f).Decode(&v); err != nil {
			return err
		}
		vars[name] = v
		return nil
	}
	err := doublestar.GlobWalk(fsys, filepath.Join(path, "**"), walkFn, doublestar.WithFilesOnly())
	if err != nil {
		return nil, err
	}
	return vars, err
}
