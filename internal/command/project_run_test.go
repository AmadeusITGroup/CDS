package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectRunCheckMutualExclusiveness(t *testing.T) {
	tests := []struct {
		name      string
		pr        projectRun
		args      []string
		expectErr bool
		errMsg    string
	}{
		{
			name:      "no conflicts",
			pr:        projectRun{},
			args:      []string{"myproject"},
			expectErr: false,
		},
		{
			name:      "path and args are mutually exclusive",
			pr:        projectRun{path: "/some/path"},
			args:      []string{"myproject"},
			expectErr: true,
			errMsg:    "the --path option and the projectName argument are mutually exclusive",
		},
		{
			name:      "path and src-repo are mutually exclusive",
			pr:        projectRun{path: "/some/path", srcRepo: "https://example.com/repo.git"},
			args:      []string{},
			expectErr: true,
			errMsg:    "the --path and the --src-repo options are mutually exclusive",
		},
		{
			name:      "src-repo and args are mutually exclusive",
			pr:        projectRun{srcRepo: "https://example.com/repo.git"},
			args:      []string{"myproject"},
			expectErr: true,
			errMsg:    "the --src-repo option and the projectName argument are mutually exclusive",
		},
		{
			name:      "override-image-tag and pull-latest are mutually exclusive",
			pr:        projectRun{overrideImageTag: "v1.0", pullLatest: true},
			args:      []string{},
			expectErr: true,
			errMsg:    kErrorImageTagFlagExclusiveness,
		},
		{
			name:      "override-image-tag and pull-given are mutually exclusive",
			pr:        projectRun{overrideImageTag: "v1.0", pullGiven: true},
			args:      []string{},
			expectErr: true,
			errMsg:    kErrorImageTagFlagExclusiveness,
		},
		{
			name:      "pull-latest and pull-given are mutually exclusive",
			pr:        projectRun{pullLatest: true, pullGiven: true},
			args:      []string{},
			expectErr: true,
			errMsg:    kErrorImageTagFlagExclusiveness,
		},
		{
			name:      "all three image tag flags are mutually exclusive",
			pr:        projectRun{overrideImageTag: "v1.0", pullLatest: true, pullGiven: true},
			args:      []string{},
			expectErr: true,
			errMsg:    kErrorImageTagFlagExclusiveness,
		},
		{
			name:      "only override-image-tag is fine",
			pr:        projectRun{overrideImageTag: "v1.0"},
			args:      []string{},
			expectErr: false,
		},
		{
			name:      "only pull-latest is fine",
			pr:        projectRun{pullLatest: true},
			args:      []string{},
			expectErr: false,
		},
		{
			name:      "only pull-given is fine",
			pr:        projectRun{pullGiven: true},
			args:      []string{},
			expectErr: false,
		},
		{
			name:      "path alone with no args is fine",
			pr:        projectRun{path: "/some/path"},
			args:      []string{},
			expectErr: false,
		},
		{
			name:      "src-repo alone with no args is fine",
			pr:        projectRun{srcRepo: "https://example.com/repo.git"},
			args:      []string{},
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.pr.checkMutualExclusiveness(tt.args)
			if tt.expectErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestProjectRunCheckCommandSemantic(t *testing.T) {
	tests := []struct {
		name      string
		pr        projectRun
		expectErr bool
	}{
		{
			name:      "no override tag — no error",
			pr:        projectRun{},
			expectErr: false,
		},
		{
			name:      "valid override tag",
			pr:        projectRun{overrideImageTag: "v1.0.0"},
			expectErr: false,
		},
		{
			name:      "invalid override tag",
			pr:        projectRun{overrideImageTag: "-invalid"},
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
