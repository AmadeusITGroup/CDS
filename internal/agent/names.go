package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// kNamePattern restricts project and container names to values that are safe
// to use as path segments and engine arguments.
var kNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func validateName(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return cerr.NewError(fmt.Sprintf("%s is required", field))
	}
	if !kNamePattern.MatchString(value) {
		return cerr.NewError(fmt.Sprintf("%s %q is invalid", field, value))
	}
	return nil
}

// validateRPCName validates a name received through the API.
func validateRPCName(field, value string) error {
	if err := validateName(field, value); err != nil {
		return status.Error(codes.InvalidArgument, cerr.Message(err))
	}
	return nil
}
