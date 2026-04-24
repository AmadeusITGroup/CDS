package containerconf

import (
	"fmt"
	"strings"
)

// dockerfileCollector emits the build dockerfile artifact, when the config
// declares one.
type dockerfileCollector struct{}

func (dockerfileCollector) Kind() string { return KindDockerfile }

func (dockerfileCollector) Collect(ctx CollectContext) ([]RequiredArtifact, error) {
	if !ctx.Config.IsSet(KBuild, KBuildDockerfile) {
		return nil, nil
	}

	dockerfilePath, ok := ctx.Config.Get(KBuild, KBuildDockerfile).(string)
	if !ok {
		return nil, fmt.Errorf("%s.%s must be a string", KBuild, KBuildDockerfile)
	}
	if strings.TrimSpace(dockerfilePath) == "" {
		return nil, fmt.Errorf("%s.%s must not be empty", KBuild, KBuildDockerfile)
	}

	source, err := resolveLocalArtifactSource(ctx.ConfigDir, dockerfilePath)
	if err != nil {
		return nil, err
	}

	identifier, err := DockerfileIdentifier(ctx.Config)
	if err != nil {
		return nil, err
	}

	return []RequiredArtifact{{Identifier: identifier, Source: source}}, nil
}
