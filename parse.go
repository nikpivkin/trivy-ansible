package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bmatcuk/doublestar/v4"
)

type ParserOption func(parser *Parser)

func WithInvertories(paths ...string) ParserOption {
	return func(parser *Parser) {
		parser.inventories = paths
	}
}

type Parser struct {
	fsys        fs.FS
	inventories []string
}

func NewParser(fsys fs.FS, opts ...ParserOption) *Parser {
	parser := &Parser{
		fsys: fsys,
	}

	for _, opt := range opts {
		opt(parser)
	}

	return parser
}

// Automatically parses Ansible projects within the given root directory,
// including nested directories. This function searches for common project
// structures and playbooks to identify and parse projects.
func (p *Parser) ParseAuto(root string) ([]*AnsibleProject, error) {
	projectPaths, err := p.autoDetectProjects(root)
	if err != nil {
		return nil, err
	}

	var projects []*AnsibleProject

	for _, projectPath := range projectPaths {
		project, err := p.parse(projectPath)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, nil
}

// Parses a specific Ansible project within the given root directory,
// using the provided playbook as the entry point. This function assumes
// a single project and parses all relevant files based on the playbook.
func (p *Parser) ParseProject(root string, playbook string) (*AnsibleProject, error) {
	project, err := p.parse(root, playbook)
	if err != nil {
		return nil, err
	}
	return project, nil
}

// Parses multiple Ansible projects within the given root directory, using
// the provided playbooks as entry points. This function handles multiple
// projects and parses each based on its respective playbook.
func (p *Parser) Parse(root string, playbooks ...string) ([]*AnsibleProject, error) {
	projects := make([]*AnsibleProject, 0, len(playbooks))
	for _, playbook := range playbooks {
		project, err := p.parse(root, playbook)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, nil
}

func (p *Parser) parse(root string, playbooks ...string) (*AnsibleProject, error) {
	project, err := p.initProject(root)
	if err != nil {
		return nil, err
	}

	if len(playbooks) == 0 {
		playbooks, err = p.resolvePlaybooksPaths(project)
		if err != nil {
			return nil, err
		}
	}

	if err := p.parsePlaybooks(project, playbooks); err != nil {
		return nil, err
	}

	parseInventories(p.fsys, project)

	return project, nil
}

func (p *Parser) initProject(root string) (*AnsibleProject, error) {
	cfg, err := readAnsibleConfig(p.fsys, root)
	if err != nil {
		return nil, fmt.Errorf("failed to read Ansible config: %w", err)
	}

	project := &AnsibleProject{
		path:       root,
		cfg:        cfg,
		dataloader: NewDataloader(p.fsys, root),
	}

	return project, nil
}

func (p *Parser) autoDetectProjects(root string) ([]string, error) {
	var res []string
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() {
			return nil
		}

		if !p.isAnsibleProject(path) {
			return nil
		}
		res = append(res, path)
		return fs.SkipDir
	}

	if err := fs.WalkDir(p.fsys, root, walkFn); err != nil {
		return nil, err
	}

	return res, nil
}

func (p *Parser) parsePlaybooks(project *AnsibleProject, paths []string) error {
	for _, path := range paths {
		playbook, err := project.dataloader.LoadPlaybook(nil, path)
		if err != nil {
			return err
		}

		if playbook == nil {
			return nil
		}

		if isMainPlaybook(path) {
			project.mainPlaybook = playbook
		} else {
			project.playbooks = append(project.playbooks, playbook)
		}
	}
	return nil
}

func (p *Parser) resolvePlaybooksPaths(project *AnsibleProject) ([]string, error) {
	entries, err := fs.ReadDir(p.fsys, project.path)
	if err != nil {
		return nil, err
	}

	var res []string

	for _, entry := range entries {
		if isYAMLFile(entry.Name()) {
			res = append(res, filepath.Join(project.path, entry.Name()))
		}
	}

	return res, nil
}

func (p *Parser) isAnsibleProject(path string) bool {
	requiredDirs := []string{
		ansibleCfgFile, "site.yml", "site.yaml", "group_vars", "host_vars", "inventory", "playbooks",
	}
	for _, filename := range requiredDirs {
		if isPathExists(p.fsys, filepath.Join(path, filename)) {
			return true
		}
	}
	entries, err := doublestar.Glob(p.fsys, "**/roles/**/{tasks,defaults,vars}")
	if err == nil && len(entries) > 0 {
		return true
	}
	return false
}

func isPathExists(fsys fs.FS, path string) bool {
	if filepath.IsAbs(path) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}
	if _, err := fs.Stat(fsys, path); err == nil {
		return true
	}
	return false
}

func isYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || ext == ".yml"
}

func cutExtension(path string) string {
	ext := filepath.Ext(path)
	return path[0 : len(path)-len(ext)]
}

func isMainPlaybook(name string) bool {
	return cutExtension(name) == "site"
}
