package agent

import (
	"fmt"
	"regexp"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var agentNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateAgentName(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", field)
	}
	if !agentNamePattern.MatchString(value) {
		return fmt.Errorf("%s %q is invalid", field, value)
	}
	return nil
}

func validateRPCName(field, value string) error {
	if err := validateAgentName(field, value); err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}
	return nil
}
