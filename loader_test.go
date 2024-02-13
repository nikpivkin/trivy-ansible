package main

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTaskWithIncludeTasks(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/test/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg:
    - "Test task"

- name: Include tasks from current role
  include_tasks:
    file: "included.yaml"

- name: Include tasks from other role
  include_tasks:
    file: "../../test2/tasks/main.yaml"
`),
		},
		"roles/test/tasks/included.yaml": {
			Data: []byte(`---
- name: Test task 2
  debug:
    msg:
    - "Test task 2"
`),
		},
		"roles/test2/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task 3
  debug:
    msg:
    - "Test task 3"
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	tasks, err := loader.LoadTasks(nil, nil, "roles/test/tasks/main.yaml")
	require.NoError(t, err)
	assert.Len(t, tasks, 3)

	flatten := tasks.Compile()

	assert.Equal(t, "Test task", flatten[0].Name())

	assert.Equal(t, "Test task 2", flatten[1].Name())
	assert.Equal(t, "Include tasks from current role", flatten[1].parent.Name())

	assert.Equal(t, "Test task 3", flatten[2].Name())
	assert.Equal(t, "Include tasks from other role", flatten[2].parent.Name())
}

func TestLoadTaskWithTplIncludeTasks(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/test/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg:
    - "Test task"

- name: Include tasks from current role
  include_tasks:
    file: '{{ includedfile }}'
  vars:
    includedfile: "included.yaml"
`),
		},
		"roles/test/tasks/included.yaml": {
			Data: []byte(`---
- name: Test task 2
  debug:
    msg:
    - "Test task 2"
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	tasks, err := loader.LoadTasks(nil, nil, "roles/test/tasks/main.yaml")
	require.NoError(t, err)
	assert.Len(t, tasks, 2)

	flatten := tasks.Compile()

	assert.Equal(t, "Test task", flatten[0].Name())

	assert.Equal(t, "Test task 2", flatten[1].Name())
	assert.Equal(t, "Include tasks from current role", flatten[1].parent.Name())

}

func TestLoadTaskWithIncludeRole(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/test/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg:
    - "Test task"

- name: Include role
  include_role:
    name: included
`),
		},
		"roles/included/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task 2
  debug:
    msg:
    - "Test task 2"
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	role, err := loader.LoadRole(nil, nil, "test")
	require.NoError(t, err)

	flatten := role.Compile().Compile()

	assert.NotEmpty(t, flatten)
	assert.Len(t, flatten, 2)

	firstTask := flatten[0]
	assert.Equal(t, "Test task", firstTask.Name())

	secondTask := flatten[1]
	assert.Equal(t, "Test task 2", secondTask.Name())
	assert.Equal(t, "Include role", secondTask.parent.Name())
}

func TestLoadTaskWithBlock(t *testing.T) {
	fsys := fstest.MapFS{
		"main.yaml": {
			Data: []byte(`---
- name: Task with block
  block:
    - name: Test task
      debug:
        msg: test task
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	tasks, err := loader.LoadTasks(nil, nil, "main.yaml")
	require.NoError(t, err)
	require.Len(t, tasks, 1)

	task := tasks[0]
	flatten := task.Compile()

	assert.Len(t, flatten, 1)
	assert.Equal(t, "Test task", flatten[0].Name())
	assert.Equal(t, "Task with block", flatten[0].parent.Name())
}

func TestLoadRoleDependencies(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/role1/meta/main.yaml": {
			Data: []byte(`---
dependencies:
- role: role2
`),
		},
		"roles/role2/tasks/main.yaml": {
			Data: []byte(`---
- name: Test task
  debug:
    msg: Test task
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	role, err := loader.LoadRole(nil, nil, "role1")
	require.NoError(t, err)

	tasks := role.Compile()
	assert.Len(t, tasks, 1)
	assert.Equal(t, "Test task", tasks[0].Name())
}

func TestIncludePlaybooks(t *testing.T) {
	fsys := fstest.MapFS{
		"playbook.yaml": {
			Data: []byte(`---
- name: Include a play after another play
  ansible.builtin.import_playbook: otherplays.yaml
`),
		},
		"otherplays.yaml": {
			Data: []byte(`---
- hosts: localhost
  tasks:
    - name: Task
      ansible.builtin.debug:
      msg: play2
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	playbook, err := loader.LoadPlaybook(nil, "playbook.yaml")
	require.NoError(t, err)

	tasks := playbook.Compile()
	assert.Len(t, tasks, 1)

	assert.Equal(t, "Task", tasks[0].Name())
}

func TestLoadPlaysWithTasks(t *testing.T) {
	fsys := fstest.MapFS{
		"playbook.yaml": {
			Data: []byte(`---
- name: Include a play after another play
  pre_tasks:
    - name: Pre task
      debug: null
      msg: Pre task
  tasks:
    - name: Task
      debug: null
      msg: Task
  post_tasks:
    - name: Post task
      debug: null
      msg: Post task
`),
		},
	}

	loader := NewDataloader(fsys, ".")
	playbook, err := loader.LoadPlaybook(nil, "playbook.yaml")
	require.NoError(t, err)

	tasks := playbook.Compile()
	assert.Len(t, tasks, 3)
	assert.Equal(t, "Pre task", tasks[0].Name())
	assert.Equal(t, "Task", tasks[1].Name())
	assert.Equal(t, "Post task", tasks[2].Name())
}
