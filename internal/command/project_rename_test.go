package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateCurrentProjectName_Empty(t *testing.T) {
	err := validateCurrentProjectName("")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "CDS is not set on a project yet")
}
