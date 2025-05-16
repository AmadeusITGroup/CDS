package ar

import (
	"fmt"
	"strings"

	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

var (
	artifactoryClients   = map[string]IArtifactoryClient{}
	artifactoryInstances = []artifactoryInstance{
		{
			name: "FixMe",                      // TODO: FixMe
			url:  "https://repository.fix.me/", // TODO: FixMe
		},
		{
			name: "local_unittesting",
			url:  "https://127.0.0.1/",
		},
	}
)

type artifactoryInstance struct {
	name string
	url  string
	err  error
}

func (ai artifactoryInstance) Name() string {
	return ai.name
}

func ArtifactoryInstanceFromName(name string) artifactoryInstance {
	for _, instance := range artifactoryInstances {
		if strings.EqualFold(name, instance.name) {
			return instance
		}
	}

	return artifactoryInstance{err: cerr.NewError(fmt.Sprintf("Failed to identify artifactory instance (%s)", name))}
}

func (ai artifactoryInstance) GetClient() IArtifactoryClient {
	if ai.err != nil {
		return &artifactoryClient{err: ai.err}
	}

	var err error
	client, ok := artifactoryClients[ai.Name()]
	if !ok {
		client, err = NewArtifactoryClient(ai.Name())
		if err != nil {
			clog.Error(fmt.Sprintf("Failed to instantiate artifactory client for artifactory instance (%s)", ai.name), err)
			return nil
		}

		artifactoryClients[ai.Name()] = client
	}

	return client
}
