package command

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/pterm/pterm"
	"gopkg.in/yaml.v3"

	"github.com/amadeusitgroup/cds/internal/bo"
	"github.com/amadeusitgroup/cds/internal/cerr"
	"github.com/amadeusitgroup/cds/internal/clog"
)

type outputFormat string

const (
	kJson outputFormat = "json"
	kYaml outputFormat = "yaml"
)

var printFormat = []outputFormat{kJson, kYaml, ""}

var cdsOutputFormat outputFormat

func filterAndConvertOutputFormat(availableFormats []outputFormat) []string {
	result := make([]string, 0, len(availableFormats))
	for _, f := range availableFormats {
		if f != "" {
			result = append(result, string(f))
		}
	}
	return result
}

func setCDSCommandOutputFormat(format string) error {
	if !slices.Contains(printFormat, outputFormat(format)) {
		return cerr.NewError(fmt.Sprintf(
			"CDS currently supports only %v as output formats. Format given was %v.",
			strings.Join(filterAndConvertOutputFormat(printFormat), ", "), format))
	}
	cdsOutputFormat = outputFormat(format)
	return nil
}

func dumpOutputFormat(data interface{}, format outputFormat) {
	var outputData []byte
	var err error
	switch format {
	case kJson:
		outputData, err = json.MarshalIndent(data, "", "  ")
	case kYaml:
		outputData, err = yaml.Marshal(&data)
	}
	if err != nil {
		clog.Error(fmt.Sprintf("Error while converting the struct into %v: %v", format, err))
		return
	}
	fmt.Println(string(outputData))
}

// formatProjectListInOutput prints the project list in the configured output format.
func formatProjectListInOutput(projectsInfo []bo.ProjectInfo, currentProject string) {
	if len(cdsOutputFormat) > 0 {
		dumpOutputFormat(projectsInfo, cdsOutputFormat)
	} else {
		printProjectList(projectsInfo, currentProject)
	}
}

func getProjectStyle(state string) *pterm.Style {
	switch state {
	case "running":
		return pterm.NewStyle(pterm.FgGreen)
	case "deleted":
		return pterm.NewStyle(pterm.FgRed)
	case "stopped":
		return pterm.NewStyle(pterm.FgYellow)
	default:
		return pterm.NewStyle(pterm.FgMagenta)
	}
}

func printProjectList(projectsInfo []bo.ProjectInfo, currentProject string) {
	fmt.Println("List of projects ('>' indicates current project, green: running, yellow: stopped, red: empty (drained or cleared), magenta: unknown):")

	bulletList := []pterm.BulletListItem{}

	for _, projectInfo := range projectsInfo {
		status := bo.GetProjStatus(projectInfo.Containers)
		style := getProjectStyle(status)

		if projectInfo.Name == currentProject {
			bulletList = append(bulletList, pterm.BulletListItem{Level: 1, Text: projectInfo.Name, Bullet: ">", BulletStyle: style})
		} else {
			bulletList = append(bulletList, pterm.BulletListItem{Level: 1, Text: projectInfo.Name, BulletStyle: style})
		}
	}

	if err := pterm.DefaultBulletList.WithItems(bulletList).Render(); err != nil {
		clog.Error("Failed to render project list", err)
	}
}
