package main

import (
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

// Role represent project role
type Role struct {
	name     string
	path     string
	pubic    bool
	metadata Metadata
	play     *Play

	tasks    []*Task
	defaults Variables
	vars     Variables
	meta     RoleMeta

	directDeps []*Role
	allDeps    []*Role

	dataloader *DataLoader
}

func (r *Role) IsPublic() bool {
	return r.pubic
}

func (r *Role) Vars() Variables {
	return r.vars
}

func (r *Role) getAllDeps() []*Role {
	if len(r.allDeps) > 0 {
		return r.allDeps
	}

	for _, dep := range r.getDirectDeps() {
		r.allDeps = append(r.allDeps, dep.getAllDeps()...)
		r.allDeps = append(r.allDeps, dep)
	}
	return r.allDeps
}

func (r *Role) getDirectDeps() []*Role {
	return r.directDeps
}

func (r *Role) loadDeps() {
	for _, dep := range r.meta.Dependencies() {
		depRole, err := r.dataloader.LoadRole(&r.meta.metadata, r.play, dep.GetName())
		if err != nil {
			panic(err) // TODO: handle error
		}
		r.directDeps = append(r.directDeps, depRole)
	}
}

func (r *Role) LoadDefaultVars() Variables {
	vars := make(Variables)
	for _, dep := range r.getAllDeps() {
		vars = lo.Assign(vars, dep.LoadDefaultVars())
	}
	return lo.Assign(vars, r.defaults)
}

// Compile returns the list of tasks for this role, which is created by first recursively
// compiling tasks for all direct dependencies and then adding tasks for this role.
func (r *Role) Compile() Tasks {

	r.loadDeps()

	var res Tasks

	for _, dep := range r.getDirectDeps() {
		res = append(res, dep.Compile()...)
	}

	for _, task := range r.tasks {
		res = append(res, task.Compile()...)
	}
	return res
}

type RoleMeta struct {
	metadata Metadata
	inner    roleMetaInner
}

func (m RoleMeta) Dependencies() []*RoleDefinition {
	return m.inner.Dependencies
}

type roleMetaInner struct {
	Dependencies []*RoleDefinition `yaml:"dependencies"`
}

func (m *RoleMeta) UnmarshalYAML(node *yaml.Node) error {
	m.metadata = Metadata{
		rng: RangeFromNode(node),
	}
	return node.Decode(&m.inner)
}
