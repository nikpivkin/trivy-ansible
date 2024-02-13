package main

import (
	"os"
	"testing"
	"testing/fstest"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseProject(t *testing.T) {
	fsys := os.DirFS("testdata/sample-proj")

	project, err := NewParser(fsys).ParseProject(".", "playbook.yaml")
	require.NoError(t, err)
	require.NotNil(t, project)

	tasks := project.ListTasks()
	assert.NotEmpty(t, tasks)
}

func TestIsAnsibleProject(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/test/tasks/main.yaml": {},
	}

	entries, err := doublestar.Glob(fsys, "**/roles/**/{tasks,defaults,vars}")
	require.NoError(t, err)
	assert.Greater(t, len(entries), 0)
}

func TestResolveTaskVariableFromPlay(t *testing.T) {
	fsys := fstest.MapFS{
		"playbook.yaml": {
			Data: []byte(`---
- hosts: localhost
  vars:
    somevar: "some_value"
  roles:
    - test
`),
		},
		"roles/test/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg: Test task
`),
		},
	}

	parser := NewParser(fsys)
	project, err := parser.ParseProject(".", "playbook.yaml")
	require.NoError(t, err)

	tasks := project.ListTasks()
	assert.Len(t, tasks, 1)

	task := tasks[0]

	variableResolver := VariableResolver{}
	vars := variableResolver.GetVars(task.Play(), task)
	assert.Equal(t, "some_value", vars["somevar"])
}

func TestOverrideTaskVariable(t *testing.T) {
	fsys := fstest.MapFS{
		"playbook.yaml": {
			Data: []byte(`---
- hosts: localhost
  vars:
    somevar: "some_value"
  roles:
    - test
`),
		},
		"roles/test/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg: Test task
  vars:
    somevar: "overrided"
`),
		},
	}

	parser := NewParser(fsys)
	project, err := parser.ParseProject(".", "playbook.yaml")
	require.NoError(t, err)

	tasks := project.ListTasks()
	assert.Len(t, tasks, 1)

	task := tasks[0]

	variableResolver := VariableResolver{}
	vars := variableResolver.GetVars(task.Play(), task)
	assert.Equal(t, "overrided", vars["somevar"])
}
