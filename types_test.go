package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUnmarshallRoleDefinition(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected RoleDefinition
	}{
		{
			name:   "role is string",
			source: `debug`,
			expected: RoleDefinition{
				metadata: Metadata{
					rng: Range{
						startLine: 1,
						endLine:   1,
					},
				},
				inner: roleDefinitionInner{
					Name: "debug",
				},
			},
		},
		{
			name:   "role is dict",
			source: `role: test`,
			expected: RoleDefinition{
				metadata: Metadata{
					rng: Range{
						startLine: 1,
						endLine:   1,
					},
				},
				inner: roleDefinitionInner{
					Name: "test",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got RoleDefinition
			err := yaml.Unmarshal([]byte(tt.source), &got)
			require.NoError(t, err)

			assert.Equal(t, tt.expected, got)
		})
	}
}
