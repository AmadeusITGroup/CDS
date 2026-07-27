package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLocalHostDeleteCommand(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "delete localhost", args: []string{"space", "host", "delete", "localhost"}, want: true},
		{name: "remove port-only local target", args: []string{"space", "host", "remove", ":8087"}, want: true},
		{name: "rm remote host", args: []string{"space", "host", "rm", "agent.example"}, want: false},
		{name: "add localhost", args: []string{"space", "host", "add", "localhost"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isLocalHostDeleteCommand(tt.args))
		})
	}
}
