package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseINIInventory(t *testing.T) {
	inventoryPath := "testdata/sample-proj/inventory/test"
	f, err := os.Open(inventoryPath)
	require.NoError(t, err)
	defer f.Close()

	inventory := parseINIInventory(f)
	_ = inventory
}

func TestParseHostVars(t *testing.T) {
	inventoryPath := "testdata/sample-proj/inventory"
	fsys := os.DirFS(inventoryPath)
	vars, err := parseHostsVars(fsys, ".")
	require.NoError(t, err)

	expected := map[string]map[string]string{
		"host1": {
			"node_name": "foo1",
		},
	}
	assert.Equal(t, expected, vars)
}
