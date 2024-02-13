package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestUnmarshallTask(t *testing.T) {
	src := []byte(`name: Create an empty bucket
amazon.aws.s3_bucket:
  name: mys3bucket
  state: present
  endpoint_url: http://localhost:4566
`)

	var task Task
	err := yaml.Unmarshal(src, &task)
	require.NoError(t, err)

	assert.Equal(t, "Create an empty bucket", task.Name())

	_, exists := task.Module("amazon.aws.s3_bucket")
	assert.True(t, exists)
}
