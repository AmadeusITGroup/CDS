package db

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/stretchr/testify/assert"
)

func Test_saveDBContent(t *testing.T) {
	tests := []struct {
		name         string
		bom          data
		expectedPath string
	}{
		{
			name:         "With a valid json",
			bom:          data{projects: projects{Projects: []*project{}}},
			expectedPath: getPathToCdsDBFile(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			sStore = &store{d: tt.bom}
			defer resetContent()

			if err := saveDBContent(); err != nil {
				t.Errorf("saveDbContent --> error = %v", err)
			}
			assert.True(t, cos.Exists(tt.expectedPath))

			file, err := cos.ReadFile(tt.expectedPath)
			if err != nil {
				t.Fatalf("Cannot read file %s: %v", tt.expectedPath, err)
			}
			expectedContent, _ := json.MarshalIndent(instance().d, "", "  ")
			assert.Equal(t, expectedContent, file)

		})
	}

}

func Test_getDBContent(t *testing.T) {
	tests := []struct {
		name              string
		bom               data
		expectedPath      string
		expectedError     error
		withFileCreation  bool
		withCorruptedFile bool
	}{
		{
			name:             "With a valid bom",
			bom:              data{Profile: profilePath{Path: getPathToDefaultProfile()}, projects: projects{Projects: []*project{{Name: "test"}}}},
			expectedPath:     getPathToCdsDBFile(),
			expectedError:    nil,
			withFileCreation: true,
		},
		{
			name:             "With a missing file",
			bom:              data{Profile: profilePath{Path: getPathToDefaultProfile()}},
			expectedPath:     getPathToCdsDBFile(),
			expectedError:    nil,
			withFileCreation: false,
		},
		{
			name:              "With a corrupted file",
			expectedPath:      getPathToCdsDBFile(),
			withFileCreation:  false,
			withCorruptedFile: true,
			expectedError:     fmt.Errorf("Failed to parse CDS database file"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			defer resetContent()

			if tt.withFileCreation {
				err := createConfigFile(tt.bom, tt.expectedPath)
				assert.Nil(t, err)
				defer func() {
					err := removeFile(tt.expectedPath)
					assert.Nil(t, err)
				}()
			} else if tt.withCorruptedFile {
				err := cos.WriteFile(tt.expectedPath, []byte("corrupted"), 0600)
				assert.Nil(t, err)
				defer func() {
					err := removeFile(tt.expectedPath)
					assert.Nil(t, err)
				}()
			}
			_, err := getDBContent()
			if err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Fatalf("getDBContent --> error = %v", err)
			}
			fmt.Println(instance().d)
			assert.True(t, reflect.DeepEqual(tt.bom, instance().d))
		})
	}
}

func Test_parseFile(t *testing.T) {
	tests := []struct {
		name          string
		path          string
		jsonContent   string
		expectedBom   data
		expectedError error
	}{
		{
			name:          "With a valid json",
			path:          getPathToCdsDBFile(),
			jsonContent:   `{"context": {"project": ""},"profile": {"path": "/dummyPath"},"projects":[]}`,
			expectedBom:   data{Context: context{ProjectContext: ""}, Profile: profilePath{Path: "/dummyPath"}, projects: projects{Projects: []*project{}}},
			expectedError: nil,
		},
		{
			name:          "With a corrupted json",
			path:          getPathToCdsDBFile(),
			jsonContent:   `corrupted`,
			expectedError: fmt.Errorf("Failed to deserialize file "),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			defer resetContent()

			err := cos.WriteFile(tt.path, []byte(tt.jsonContent), 0600)
			assert.Nil(t, err)
			defer func() {
				err := removeFile(tt.path)
				assert.Nil(t, err)
			}()

			sStore = newDB()
			defer resetContent()

			err = parseFile(tt.path, sStore)
			if err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Fatalf("parseFile --> error = %v", err)
			}
			assert.True(t, reflect.DeepEqual(tt.expectedBom, sStore.d))

		})
	}

}

func Test_instance(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "With a valid instance",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer resetContent()
			instance()
			assert.NotNil(t, sStore)
		})
	}
}

func Test_Load(t *testing.T) {
	tests := []struct {
		name          string
		expectedBom   data
		path          string
		expectedError error
		jsonContent   string
	}{
		{
			name:          "With a valid json",
			path:          getPathToCdsDBFile(),
			jsonContent:   `{"context": {"project": ""},"profile": {"path": "/dummyPath"},"projects":[]}`,
			expectedBom:   data{Context: context{ProjectContext: ""}, Profile: profilePath{Path: "/dummyPath"}, projects: projects{Projects: []*project{}}},
			expectedError: nil,
		},
		{
			name:          "With a corrupted file",
			path:          getPathToCdsDBFile(),
			jsonContent:   `corrupted`,
			expectedError: fmt.Errorf("Failed to initialize cds configuration"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			defer resetContent()

			err := cos.WriteFile(tt.path, []byte(tt.jsonContent), 0600)
			assert.Nil(t, err)
			defer func() {
				err := removeFile(tt.path)
				assert.Nil(t, err)
			}()
			err = Load()
			if err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Fatalf("Load --> error = %v", err)
			}
			assert.True(t, reflect.DeepEqual(tt.expectedBom, instance().d))
		})
	}
}

func Test_Save(t *testing.T) {
	tests := []struct {
		name          string
		bom           data
		expectedPath  string
		expectedError error
	}{
		{
			name:          "With a valid bom",
			bom:           data{Profile: profilePath{Path: getPathToDefaultProfile()}, projects: projects{Projects: []*project{{Name: "test"}}}},
			expectedPath:  getPathToCdsDBFile(),
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			sStore = &store{d: tt.bom}
			defer resetContent()

			if err := Save(); err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Errorf("saveDbContent --> error = %v", err)
			}
			assert.True(t, cos.Exists(tt.expectedPath))

			file, err := cos.ReadFile(tt.expectedPath)
			if err != nil {
				t.Fatalf("Cannot read file %s: %v", tt.expectedPath, err)
			}
			expectedContent, _ := json.MarshalIndent(instance().d, "", "  ")
			assert.Equal(t, expectedContent, file)

		})
	}
}
