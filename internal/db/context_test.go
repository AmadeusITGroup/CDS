package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_FlushContext(t *testing.T) {
	tests := []struct {
		name string
		bom  data
	}{
		{
			name: "Flush context with a project set",
			bom: data{
				Context:  context{ProjectContext: "myProject"},
				projects: projects{Projects: []*project{{Name: "myProject"}}},
			},
		},
		{
			name: "Flush context when already empty",
			bom: data{
				Context:  context{ProjectContext: ""},
				projects: projects{Projects: []*project{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tearDown := setupTest(t, tt.bom)
			defer tearDown()

			err := FlushContext()
			assert.Nil(t, err)
			assert.Equal(t, "", GetCurrentProject())
		})
	}
}

func Test_FlushContext_ClearsExistingProject(t *testing.T) {
	bom := data{
		Context:  context{ProjectContext: "proj1"},
		projects: projects{Projects: []*project{{Name: "proj1"}}},
	}

	tearDown := setupTest(t, bom)
	defer tearDown()

	// Verify project is set before flush.
	assert.Equal(t, "proj1", GetCurrentProject())
	assert.True(t, IsCurrentProject("proj1"))

	err := FlushContext()
	assert.Nil(t, err)

	// After flush, no project should be selected.
	assert.Equal(t, "", GetCurrentProject())
	assert.False(t, IsCurrentProject("proj1"))
}
