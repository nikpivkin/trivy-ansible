package main

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

type DataLoader struct {
	fsys fs.FS
	root string

	// The cache key is the role name
	// The cache value is the path to the role definition directory
	roleCache map[string]string
}

func NewDataloader(fsys fs.FS, root string) *DataLoader {
	return &DataLoader{
		fsys:      fsys,
		root:      root,
		roleCache: make(map[string]string),
	}
}

// TODO: add public field
type LoadRoleOptions struct {
	TasksFile    string
	DefaultsFile string
	VarsFile     string
	Public       *bool
}

func (o LoadRoleOptions) WithDefaults() LoadRoleOptions {
	res := LoadRoleOptions{
		TasksFile:    "main",
		DefaultsFile: "main",
		VarsFile:     "main",
	}

	if o.TasksFile != "" {
		res.TasksFile = o.TasksFile
	}

	if o.DefaultsFile != "" {
		res.DefaultsFile = o.DefaultsFile
	}

	if o.VarsFile != "" {
		res.VarsFile = o.VarsFile
	}

	return res
}

func (l *DataLoader) LoadRole(meta *Metadata, play *Play, roleName string) (*Role, error) {
	return l.LoadRoleWithOptions(meta, play, roleName, LoadRoleOptions{})
}

func (l *DataLoader) LoadRoleWithOptions(meta *Metadata, play *Play, roleName string, opt LoadRoleOptions) (*Role, error) {
	opt = opt.WithDefaults()

	var rolePath string
	// TODO: For caching it is necessary to load the role completely, so as not to depend on LoadRoleOptions
	// if val, exists := l.roleCache[roleName]; exists {
	// 	val.play = play
	// 	val.metadata.parent = meta
	// 	return &val, nil
	// }

	if val, exists := l.roleCache[roleName]; exists {
		rolePath = val
	} else if val, exists := l.resolveRolePath(roleName); exists {
		rolePath = val
	}

	if rolePath == "" {
		return nil, fmt.Errorf("role %q not found", roleName)
	}

	r := &Role{
		name: roleName,
		path: rolePath,
		metadata: Metadata{
			parent: meta,
			path:   rolePath,
		},
		play:       play,
		dataloader: l,
	}

	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		dir, filename := filepath.Split(path)
		if !isYAMLFile(filename) {
			return nil
		}

		parts := strings.Split(dir, string(os.PathSeparator))
		parentFolder := parts[len(parts)-2]

		switch parentFolder {
		case "tasks":
			if cutExtension(filename) != opt.TasksFile {
				return nil
			}
			tasks, err := l.LoadTasks(&r.metadata, r, path)
			if err != nil {
				return fmt.Errorf("failed to load tasks: %w", err)
			}

			r.tasks = append(r.tasks, tasks...)
		case "defaults":
			if cutExtension(filename) != opt.DefaultsFile {
				return nil
			}
			if vars, err := l.parseVarsFile(path); err == nil {
				r.defaults = lo.Assign(r.defaults, vars)
			}
		case "vars":
			if cutExtension(filename) != opt.VarsFile {
				return nil
			}
			if vars, err := l.parseVarsFile(path); err == nil {
				r.vars = lo.Assign(r.vars, vars)
			}
		case "meta":
			if cutExtension(filename) != "main" {
				return nil
			}
			if meta, err := l.parseMetaFile(path); err == nil {
				meta.metadata.parent = &r.metadata
				r.meta = meta
			}
		}
		return nil
	}
	if err := fs.WalkDir(l.fsys, rolePath, walkFn); err != nil {
		return nil, err
	}

	l.roleCache[roleName] = rolePath

	return r, nil
}

func (l *DataLoader) parseMetaFile(path string) (RoleMeta, error) {
	var meta RoleMeta
	if err := l.decodeYAMLFile(path, &meta); err != nil {
		return meta, err
	}
	meta.metadata.path = path
	return meta, nil
}

func (l *DataLoader) parseVarsFile(path string) (map[string]any, error) {
	var vars map[string]any
	if err := l.decodeYAMLFile(path, &vars); err != nil {
		return nil, err
	}
	return vars, nil
}

func (l *DataLoader) decodeYAMLFile(path string, dst any) error {
	f, err := l.fsys.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return yaml.NewDecoder(f).Decode(dst)
}

func (l *DataLoader) resolveRolePath(name string) (string, bool) {
	paths := []string{filepath.Join(l.root, "roles", name)}
	if defaultRolesPath, exists := os.LookupEnv("DEFAULT_ROLES_PATH"); exists {
		paths = append(paths, defaultRolesPath)
	}

	for _, p := range paths {
		if isPathExists(l.fsys, p) {
			return p, true
		}
	}

	return "", false
}

func (l *DataLoader) LoadTasks(sourceMetadata *Metadata, role *Role, path string) (Tasks, error) {
	var tasks Tasks
	if err := l.decodeYAMLFile(path, &tasks); err != nil {
		return nil, fmt.Errorf("failed to decode tasks file %q: %w", path, err)
	}
	tasks = lo.Map(tasks, func(task *Task, _ int) *Task {
		task.metadata.parent = sourceMetadata
		task.dataloader = l
		task.role = role
		task.UpdateNested(path)
		return task
	})

	return tasks, nil
}

func (l *DataLoader) LoadPlaybook(sourceMetadata *Metadata, path string) (Playbook, error) {

	f, err := l.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var playbook Playbook
	if err := yaml.NewDecoder(f).Decode(&playbook); err != nil {
		// not all YAML files are playbooks.
		log.Printf("Failed to decode playbook %q: %s", path, err)
		return nil, nil
	}
	for _, play := range playbook {
		play.UpdateMetadata(sourceMetadata, path)
		play.dataloader = l

		roles := make([]*Role, 0, len(play.GetRoleDefinitions()))

		for _, roleDef := range play.GetRoleDefinitions() {
			role, err := l.LoadRole(&play.metadata, play, roleDef.GetName())
			if err != nil {
				return nil, fmt.Errorf("failed to load role %q: %w", roleDef.GetName(), err)
			}
			roles = append(roles, role)
		}
		play.roles = roles
	}
	return playbook, nil
}

func (l *DataLoader) LoadPlayVarsFile(playPath string, varsFile string) (map[string]any, error) {
	path := filepath.Join(playPath, "vars", varsFile)
	f, err := l.fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var vars map[string]any
	if err := yaml.NewDecoder(f).Decode(&vars); err != nil {
		return nil, fmt.Errorf("failed to decode variables from %q: %w", path, err)
	}

	return vars, nil
}
