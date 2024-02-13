package main

import (
	"fmt"

	"github.com/flosch/pongo2/v6"
)

type Templater struct{}

func (t *Templater) Evaluate(variable string, vars Variables) (string, error) {
	tpl, err := pongo2.FromString(variable)
	if err != nil {
		return "", fmt.Errorf("failed to load template from %q: %w", variable, err)
	}
	out, err := tpl.Execute(pongo2.Context(vars))
	if err != nil {
		return "", fmt.Errorf("failed to execute template %q: %w", variable, err)
	}
	return out, nil
}
