package db

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/amadeusitgroup/cds/internal/cos"
	"github.com/amadeusitgroup/cds/internal/source"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
)

// initTestSource sets up dbSrc pointing to testDBPath on the current mock FS.
func initTestSource(t *testing.T) {
	t.Helper()
	src, err := source.NewLocalSource(testDBPath)
	if err != nil {
		t.Fatalf("failed to create test source: %v", err)
	}
	dbSrc = src
}

func Test_saveDBContent(t *testing.T) {
	tests := []struct {
		name string
		bom  data
	}{
		{
			name: "With a valid json",
			bom:  data{projects: projects{Projects: []*project{}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.SetMockedFileSystem()
			defer cos.SetRealFileSystem()
			sStore = &store{d: tt.bom}
			defer resetContent()

			initTestSource(t)
			if err := saveDBContent(); err != nil {
				t.Errorf("saveDbContent --> error = %v", err)
			}
			assert.True(t, cos.Exists(testDBPath))

			file, err := cos.ReadFile(testDBPath)
			if err != nil {
				t.Fatalf("Cannot read file %s: %v", testDBPath, err)
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
		expectedError     error
		withFileCreation  bool
		withCorruptedFile bool
	}{
		{
			name:             "With a valid bom",
			bom:              data{projects: projects{Projects: []*project{{Name: "test"}}}},
			expectedError:    nil,
			withFileCreation: true,
		},
		{
			name:             "With a missing file",
			bom:              data{},
			expectedError:    nil,
			withFileCreation: false,
		},
		{
			name:              "With a corrupted file",
			withFileCreation:  false,
			withCorruptedFile: true,
			expectedError:     fmt.Errorf("Failed to parse CDS database file"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.Fs = afero.NewMemMapFs()
			defer cos.SetRealFileSystem()
			defer resetContent()

			initTestSource(t)

			if tt.withFileCreation {
				content, _ := json.Marshal(tt.bom)
				err := cos.WriteFile(testDBPath, content, 0600)
				assert.Nil(t, err)
			} else if tt.withCorruptedFile {
				err := cos.WriteFile(testDBPath, []byte("corrupted"), 0600)
				assert.Nil(t, err)
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

func Test_parseSource(t *testing.T) {
	tests := []struct {
		name          string
		jsonContent   string
		expectedBom   data
		expectedError error
	}{
		{
			name:          "With a valid json",
			jsonContent:   `{"context": {"project": ""},"projects":[]}`,
			expectedBom:   data{Context: context{ProjectContext: ""}, projects: projects{Projects: []*project{}}},
			expectedError: nil,
		},
		{
			name:          "With a corrupted json",
			jsonContent:   `corrupted`,
			expectedError: fmt.Errorf("Failed to deserialize source"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.Fs = afero.NewMemMapFs()
			defer cos.SetRealFileSystem()
			defer resetContent()

			err := cos.WriteFile(testDBPath, []byte(tt.jsonContent), 0600)
			assert.Nil(t, err)

			initTestSource(t)
			sStore = newDB()
			defer resetContent()

			err = parseSource(sStore)
			if err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Fatalf("parseSource --> error = %v", err)
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

func Test_Init(t *testing.T) {
	tests := []struct {
		name          string
		expectedBom   data
		expectedError error
		jsonContent   string
	}{
		{
			name:          "With a valid json",
			jsonContent:   `{"context": {"project": ""},"projects":[]}`,
			expectedBom:   data{Context: context{ProjectContext: ""}, projects: projects{Projects: []*project{}}},
			expectedError: nil,
		},
		{
			name:          "With a corrupted file",
			jsonContent:   `corrupted`,
			expectedError: fmt.Errorf("Failed to initialize cds configuration"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.Fs = afero.NewMemMapFs()
			defer cos.SetRealFileSystem()
			defer resetContent()

			err := cos.WriteFile(testDBPath, []byte(tt.jsonContent), 0600)
			assert.Nil(t, err)

			initTestSource(t)
			err = Load(dbSrc)
			if err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Fatalf("Init --> error = %v", err)
			}
			assert.True(t, reflect.DeepEqual(tt.expectedBom, instance().d))
		})
	}
}

func Test_Save(t *testing.T) {
	tests := []struct {
		name          string
		bom           data
		expectedError error
	}{
		{
			name:          "With a valid bom",
			bom:           data{projects: projects{Projects: []*project{{Name: "test"}}}},
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cos.Fs = afero.NewMemMapFs()
			defer cos.SetRealFileSystem()
			sStore = &store{d: tt.bom}
			defer resetContent()

			initTestSource(t)
			if err := Save(); err != nil {
				if tt.expectedError != nil {
					assert.True(t, strings.Contains(err.Error(), tt.expectedError.Error()))
					return
				}
				t.Errorf("Save --> error = %v", err)
			}
			assert.True(t, cos.Exists(testDBPath))

			file, err := cos.ReadFile(testDBPath)
			if err != nil {
				t.Fatalf("Cannot read file %s: %v", testDBPath, err)
			}
			expectedContent, _ := json.MarshalIndent(instance().d, "", "  ")
			assert.Equal(t, expectedContent, file)
		})
	}
}
