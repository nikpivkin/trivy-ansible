package main

import (
	"log"
	"path/filepath"

	"github.com/mitchellh/mapstructure"
	"github.com/samber/lo"
	"gopkg.in/yaml.v3"
)

const (
	ansibleBuiltinPrefix = "ansible.builtin."

	includeRoleAction  = "include_role"
	importRoleAction   = "import_role"
	includeTasksAction = "include_tasks"
	importTasksAction  = "import_tasks"
)

func applyBuiltinPrefix(action string) string {
	return ansibleBuiltinPrefix + action
}

func applyBuiltinPrefixAll(actions ...string) []string {
	return append(actions, lo.Map(actions, func(action string, _ int) string {
		return applyBuiltinPrefix(action)
	})...)
}

type Module map[string]any

func (m Module) ToStringMap() map[string]string {
	res := make(map[string]string)
	for k, v := range m {
		strVal, ok := v.(string)
		if ok {
			res[k] = strVal
		}
	}
	return res
}

type Tasks []*Task

// Compile expands and compiles this collection of tasks, returning a flattened
// list of all resulting tasks. Each Task within the collection is compiled
// recursively, producing its constituent tasks, which are then appended to the
// result slice.
func (t Tasks) Compile() Tasks {
	var res Tasks
	for _, task := range t {
		res = append(res, task.Compile()...)
	}
	return res
}

// Task represents a single task within an Ansible playbook. It contains information
// about the task's definition, context, and associated metadata. Tasks can be further
// composed of subtasks or reference external modules and roles.
type Task struct {
	inner    taskInner
	metadata Metadata
	role     *Role
	play     *Play
	parent   *Task

	varResolver *VariableResolver
	templater   *Templater

	raw        map[string]any
	dataloader *DataLoader

	cachedVars Variables
}

type taskInner struct {
	Name  string    `yaml:"name"`
	Block []*Task   `yaml:"block"`
	Vars  Variables `yaml:"vars"`
}

func (t *Task) GetMetadata() Metadata {
	return t.metadata
}

func (t *Task) Name() string {
	return t.inner.Name
}

func (t *Task) Role() *Role {
	return t.role
}

func (t *Task) Play() *Play {
	if t.role != nil {
		return t.role.play
	}
	return t.play
}

func (t *Task) Vars() Variables {
	return t.inner.Vars
}

func (t *Task) UpdateNested(path string) {
	t.metadata.path = path
	for _, b := range t.inner.Block {
		b.metadata.path = path
		b.dataloader = t.dataloader
		b.role = t.role
	}
}

func (t *Task) updateParent(parent *Task) {
	t.parent = parent
	t.metadata.parent = &parent.metadata
}

func (t *Task) UnmarshalYAML(node *yaml.Node) error {
	t.metadata = Metadata{
		rng: RangeFromNode(node),
	}

	var rawMap map[string]any
	if err := node.Decode(&rawMap); err != nil {
		return err
	}

	t.raw = rawMap
	if err := node.Decode(&t.inner); err != nil {
		return err
	}
	for _, b := range t.inner.Block {
		b.updateParent(t)
	}
	return nil
}

// isModuleFreeForm determines whether a module parameter is defined as a free-form
// string value within the task's raw data.
//
// Example:
// - include_tasks: file.yml
func (t *Task) isModuleFreeForm(moduleName string) (string, bool) {
	param, exists := t.raw[moduleName]
	if !exists {
		return "", false
	}

	if _, ok := param.(string); !ok {
		return "", false
	}

	vars := t.varResolver.GetVars(t.Play(), t)

	rendered, err := t.renderVariable(param, vars)
	if err != nil {
		log.Printf("Failed to render variable: %s", err)
		return "", false
	}

	val, ok := rendered.(string)
	if !ok {
		return "", false
	}

	return val, true
}

func (t *Task) Module(moduleName string) (Module, bool) {
	val, exists := t.raw[moduleName]
	if !exists {
		return nil, false
	}
	params, ok := val.(map[string]any)
	if !ok {
		return nil, false
	}

	// TODO: should variables be cached?
	if t.cachedVars == nil {
		t.cachedVars = t.varResolver.GetVars(t.Play(), t)
	}

	module := make(Module, len(params))

	for name, param := range params {
		rendered, err := t.renderVariable(param, t.cachedVars)
		if err != nil {
			log.Printf("Failed to render variable: %s", err)
			return nil, false
		}
		module[name] = rendered
	}

	return module, true
}

func (t *Task) renderVariable(variable any, vars Variables) (any, error) {
	switch v := variable.(type) {
	case string:
		rendered, err := t.templater.Evaluate(v, vars)
		if err != nil {
			return "", err
		}
		return rendered, nil
	case []any:
		res := make([]any, 0, len(v))
		for _, vv := range v {
			rendered, err := t.renderVariable(vv, vars)
			if err != nil {
				return "", err
			}
			res = append(res, rendered)
		}
		return res, nil
	case map[string]any:
		res := make(map[string]any, len(v))
		for k, vv := range v {
			rendered, err := t.renderVariable(vv, vars)
			if err != nil {
				return "", err
			}
			res[k] = rendered
		}
		return res, nil
	}
	log.Printf("Unsupported variable type: %T", variable)
	return variable, nil
}

func (t *Task) isTaskInclude() bool {
	return t.actionOneOf(applyBuiltinPrefixAll(importTasksAction, includeTasksAction))
}

func (t *Task) isRoleInclude() bool {
	return t.actionOneOf(applyBuiltinPrefixAll(importRoleAction, includeRoleAction))
}

func (t *Task) actionOneOf(actions []string) bool {
	for _, action := range actions {
		_, exists := t.raw[action]
		if exists {
			return true
		}
	}
	return false
}

func (t *Task) IsBlock() bool {
	return len(t.inner.Block) > 0
}

// RoleIncludeModule represents the "include_role" or "import_role" module
type RoleIncludeModule struct {
	Name         string `mapstruct:"name"`
	TasksFrom    string `mapstruct:"tasks_from"`
	DefaultsFrom string `mapstruct:"defaults_from"`
	VarsFrom     string `mapstruct:"vars_from"`
	Public       bool   `mapstruct:"public"`
}

// TaskIncludeModule represents the "include_tasks" or "import_tasks" module
type TaskInclude struct {
	File string `mapstruct:"file"`
}

// Compile recursively compiles the current task and its subtasks, returning a
// list of all resulting tasks. The behavior depends on the task type:
//   - Block tasks: Each subtask in the block is compiled and its results are
//     appended. Parent information is updated.
//   - Include tasks: The specified tasks file is loaded and its tasks are compiled,
//     updating parent information.
//   - Role include tasks: The specified role is loaded with options, and its compiled
//     tasks are added, again updating parent information.
//   - Other tasks: The current task is returned as a single-element list.
func (t *Task) Compile() Tasks {
	switch {
	case len(t.inner.Block) > 0:
		return t.compileBlockTasks()
	case t.isTaskInclude():
		return t.compileTaskInclude()
	case t.isRoleInclude():
		return t.compileRoleInclude()
	default:
		return Tasks{t}
	}
}

func (t *Task) compileBlockTasks() Tasks {
	var res []*Task
	for _, task := range t.inner.Block {
		// task.updateParent(t)
		res = append(res, task.Compile()...)
	}
	return res
}

func (t *Task) compileTaskInclude() Tasks {
	var res []*Task

	rawModule := make(map[string]string)
	for _, action := range applyBuiltinPrefixAll(includeTasksAction, importTasksAction) {
		if val, ok := t.isModuleFreeForm(action); ok {
			rawModule["file"] = val
		} else if val, ok := t.Module(action); ok {
			rawModule = val.ToStringMap()
		}
	}

	var module TaskInclude
	if err := mapstructure.Decode(rawModule, &module); err != nil {
		panic(err) // TODO: handle error
	}

	// TODO: the task path can be absolute
	tasksFile := filepath.Join(filepath.Dir(t.metadata.path), module.File)

	loadedTasks, err := t.dataloader.LoadTasks(&t.metadata, t.role, tasksFile)
	if err != nil {
		panic(err) // TODO: handle error
	}

	for _, task := range loadedTasks {
		task.updateParent(t)
		res = append(res, task.Compile()...)
	}

	return res
}

func (t *Task) compileRoleInclude() Tasks {
	var res []*Task

	rawModule := make(map[string]string)
	for _, action := range applyBuiltinPrefixAll(includeRoleAction, importRoleAction) {
		if val, ok := t.isModuleFreeForm(action); ok {
			rawModule["file"] = val
		} else if val, ok := t.Module(action); ok {
			rawModule = val.ToStringMap()
		}
	}

	var module RoleIncludeModule
	if err := mapstructure.Decode(rawModule, &module); err != nil {
		panic(err) // TODO: handle error
	}

	r, err := t.dataloader.LoadRoleWithOptions(&t.metadata, t.role.play, module.Name, LoadRoleOptions{
		TasksFile:    module.TasksFrom,
		DefaultsFile: module.Name,
		VarsFile:     module.VarsFrom,
	})

	if err != nil {
		panic(err) // TODO: handle error
	}
	for _, task := range r.Compile() {
		// TODO: do not update the parent in the metadata here, as the dependency chain may be lost
		// if the task is a role dependency task
		task.updateParent(t)
		res = append(res, task)
	}

	return res
}
