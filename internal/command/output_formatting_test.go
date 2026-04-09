package command

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/amadeusitgroup/cds/internal/bo"
)

func TestPrintProjectInfo(t *testing.T) {
	tests := []struct {
		name        string
		projectInfo bo.ProjectInfo
	}{
		{
			name: "Project with running containers",
			projectInfo: bo.ProjectInfo{
				Name: "testProject",
				Host: "localhost",
				Containers: []bo.ContainerInfo{
					{Id: "1", Name: "container1", Status: "running"},
					{Id: "2", Name: "container2", Status: "running"},
				},
			},
		},
		{
			name: "Project with no containers",
			projectInfo: bo.ProjectInfo{
				Name: "emptyProject",
				Host: "remotehost",
			},
		},
		{
			name: "Project with deleted containers filtered out",
			projectInfo: bo.ProjectInfo{
				Name: "mixedProject",
				Host: "myhost",
				Containers: []bo.ContainerInfo{
					{Id: "1", Name: "active", Status: "running"},
					{Id: "2", Name: "gone", Status: "deleted"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// printProjectInfo writes to stdout via pterm; verify it does not panic.
			assert.NotPanics(t, func() {
				printProjectInfo(tt.projectInfo)
			})
		})
	}
}

func TestFormatProjectInfoInOutput_JSON(t *testing.T) {
	old := cdsOutputFormat
	defer func() { cdsOutputFormat = old }()

	cdsOutputFormat = kJson

	// Should not panic when dumping as JSON.
	assert.NotPanics(t, func() {
		formatProjectInfoInOutput(bo.ProjectInfo{
			Name: "proj",
			Host: "host1",
			Containers: []bo.ContainerInfo{
				{Id: "1", Name: "c1", Status: "running"},
			},
		})
	})
}

func TestFormatProjectInfoInOutput_YAML(t *testing.T) {
	old := cdsOutputFormat
	defer func() { cdsOutputFormat = old }()

	cdsOutputFormat = kYaml

	assert.NotPanics(t, func() {
		formatProjectInfoInOutput(bo.ProjectInfo{
			Name: "proj",
			Host: "host1",
		})
	})
}

func TestFormatProjectInfoInOutput_Default(t *testing.T) {
	old := cdsOutputFormat
	defer func() { cdsOutputFormat = old }()

	cdsOutputFormat = ""

	// Falls through to printProjectInfo.
	assert.NotPanics(t, func() {
		formatProjectInfoInOutput(bo.ProjectInfo{
			Name: "proj",
			Host: "host1",
		})
	})
}
