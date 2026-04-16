package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectRebuildCheckMutualExclusiveness(t *testing.T) {
	tests := []struct {
		name      string
		pr        projectRebuild
		expectErr bool
	}{
		{
			name:      "no flags set — no error",
			pr:        projectRebuild{},
			expectErr: false,
		},
		{
			name:      "only override-image-tag",
			pr:        projectRebuild{overrideImageTag: "v2.0"},
			expectErr: false,
		},
		{
			name:      "only pull-latest",
			pr:        projectRebuild{pullLatest: true},
			expectErr: false,
		},
		{
			name:      "only pull-given",
			pr:        projectRebuild{pullGiven: true},
			expectErr: false,
		},
		{
			name:      "override-image-tag and pull-latest conflict",
			pr:        projectRebuild{overrideImageTag: "v2.0", pullLatest: true},
			expectErr: true,
		},
		{
			name:      "override-image-tag and pull-given conflict",
			pr:        projectRebuild{overrideImageTag: "v2.0", pullGiven: true},
			expectErr: true,
		},
		{
			name:      "pull-latest and pull-given conflict",
			pr:        projectRebuild{pullLatest: true, pullGiven: true},
			expectErr: true,
		},
		{
			name:      "all three flags conflict",
			pr:        projectRebuild{overrideImageTag: "v2.0", pullLatest: true, pullGiven: true},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pr.checkMutualExclusiveness()
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), kErrorImageTagFlagExclusiveness)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProjectRebuildCheckCommandSemantic(t *testing.T) {
	tests := []struct {
		name      string
		pr        projectRebuild
		expectErr bool
	}{
		{
			name:      "no override tag — no error",
			pr:        projectRebuild{},
			expectErr: false,
		},
		{
			name:      "valid override tag",
			pr:        projectRebuild{overrideImageTag: "1.2.3"},
			expectErr: false,
		},
		{
			name:      "invalid override tag with colon",
			pr:        projectRebuild{overrideImageTag: "1:2"},
			expectErr: true,
		},
		{
			name:      "invalid override tag starting with dash",
			pr:        projectRebuild{overrideImageTag: "-bad"},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pr.checkCommandSemantic()
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
