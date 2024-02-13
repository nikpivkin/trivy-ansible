package main

import (
	"gopkg.in/yaml.v3"
)

type Variables map[string]any

type AnsibleProject struct {
	path string

	cfg AnsibleConfig
	// inventory Inventory
	mainPlaybook Playbook
	playbooks    []Playbook

	dataloader *DataLoader
}

func (p *AnsibleProject) ListTasks() Tasks {
	var res Tasks
	if p.mainPlaybook != nil {
		res = append(res, p.mainPlaybook.Compile()...)
	} else {
		for _, playbook := range p.playbooks {
			res = append(res, playbook.Compile()...)
		}
	}
	return res
}

type Playbook []*Play

func (p Playbook) Compile() Tasks {
	var res Tasks
	for _, play := range p {
		res = append(res, play.Compile()...)
	}
	return res
}

// TODO: support for "module_defaults"
// https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_module_defaults.html
type Play struct {
	metadata Metadata
	raw      map[string]any

	roles      []*Role
	dataloader *DataLoader
	inner      playInner
}

func (p *Play) GetPath() string {
	return p.metadata.path
}

func (p *Play) GetRoles() []*Role {
	return p.roles
}

type playInner struct {
	Name            string            `yaml:"name"`
	ImportPlaybook  string            `yaml:"import_playbook"`
	Hosts           string            `yaml:"hosts"`
	RoleDefinitions []*RoleDefinition `yaml:"roles"`
	PreTasks        []*Task           `yaml:"pre_tasks"`
	Tasks           []*Task           `yaml:"tasks"`
	PostTasks       []*Task           `yaml:"post_tasks"`
	Vars            Variables         `yaml:"vars"`
	VarFiles        []string          `yaml:"var_files"`
}

func (p *Play) GetMetadata() Metadata {
	return p.metadata
}

func (p *Play) GetVars() Variables {
	return p.inner.Vars
}

func (p *Play) GetVarsFiles() []string {
	return p.inner.VarFiles
}

func (p *Play) GetRoleDefinitions() []*RoleDefinition {
	return p.inner.RoleDefinitions
}

func (p *Play) UnmarshalYAML(node *yaml.Node) error {
	p.metadata = Metadata{
		rng: RangeFromNode(node),
	}
	if err := node.Decode(&p.raw); err != nil {
		return err
	}
	return node.Decode(&p.inner)
}

func (p *Play) UpdateMetadata(parent *Metadata, path string) {
	p.metadata.parent = parent
	p.metadata.path = path
	for _, roleDef := range p.inner.RoleDefinitions {
		roleDef.metadata.path = path
		roleDef.metadata.parent = &p.metadata
	}

	for _, task := range p.listTasks() {
		task.metadata.path = path
		task.metadata.parent = &p.metadata
	}
}

// Compile compiles and returns the task list for this play, compiled from the
// roles (which are themselves compiled recursively) and/or the list of
// tasks specified in the play.
func (p *Play) Compile() Tasks {
	var res Tasks
	if playbookPath, ok := p.isIncludePlaybook(); ok {
		included, err := p.dataloader.LoadPlaybook(&p.metadata, playbookPath)
		if err != nil {
			panic(err) // TODO: handle error
		}
		return included.Compile()
	}

	for _, task := range p.listTasks() {
		res = append(res, task.Compile()...)
	}

	for _, role := range p.roles {
		res = append(res, role.Compile()...)
	}

	return res
}

func (p *Play) listTasks() Tasks {
	res := make(Tasks, 0, len(p.inner.PreTasks)+len(p.inner.Tasks)+len(p.inner.PostTasks))
	res = append(res, p.inner.PreTasks...)
	res = append(res, p.inner.Tasks...)
	res = append(res, p.inner.PostTasks...)
	return res
}

// TODO support collections
// ansible.builtin.import_playbook: my_namespace.my_collection.my_playbook
func (p *Play) isIncludePlaybook() (string, bool) {
	for _, k := range applyBuiltinPrefixAll("import_playbook", "include_playbook") {
		val, exists := p.raw[k]
		if !exists {
			continue
		}
		// TODO: render tpl
		playbookPath, ok := val.(string)
		return playbookPath, ok
	}

	return "", false
}

type RoleDefinition struct {
	metadata Metadata
	inner    roleDefinitionInner
}

type roleDefinitionInner struct {
	Name string         `yaml:"role"`
	Vars map[string]any `yaml:"vars"`
}

func (r *RoleDefinition) UnmarshalYAML(node *yaml.Node) error {
	r.metadata = Metadata{
		rng: RangeFromNode(node),
	}

	// a role can be a string or a dictionary
	if node.Kind == yaml.ScalarNode {
		r.inner.Name = node.Value
		return nil
	}

	return node.Decode(&r.inner)
}
func (r *RoleDefinition) GetName() string {
	return r.inner.Name
}
