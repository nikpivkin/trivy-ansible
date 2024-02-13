package main

import "github.com/samber/lo"

type VariableResolver struct{}

// TODO: pass Host
/*
The order of precedence is:
	- play->roles->get_default_vars (if there is a play context)
	- group_vars_files[host] (if there is a host context)
	- host_vars_files[host] (if there is a host context)
	- host->get_vars (if there is a host context)
	- fact_cache[host] (if there is a host context)
	- play vars (if there is a play context)
	- play vars_files (if there's no host context, ignore
		file names that cannot be templated)
	- task->get_vars (if there is a task context)
	- vars_cache[host] (if there is a host context)
	- extra vars

See https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_variables.html#variable-precedence-where-should-i-put-a-variable
*/
func (r *VariableResolver) GetVars(play *Play, task *Task) Variables {
	res := make(Variables)

	if play != nil {
		for _, role := range play.GetRoles() {
			// TODO: check if role public
			res = lo.Assign(res, role.LoadDefaultVars())
		}
	}

	if play != nil {
		res = lo.Assign(res, play.GetVars())

		for _, varsFile := range play.GetVarsFiles() {
			f, err := play.dataloader.LoadPlayVarsFile(play.GetPath(), varsFile)
			if err != nil {
				panic(err) // TODO: handle error
			}
			res = lo.Assign(res, f)
		}

		for _, role := range play.GetRoles() {
			res = lo.Assign(res, role.Vars())
		}
	}

	if task != nil {
		if task.Role() != nil {
			res = lo.Assign(res, task.Role().Vars())
		}
		res = lo.Assign(res, task.Vars())
	}

	return res
}
