package cg

import (
	"encoding/json"
	"os"
)

func UnmarshalJSON(path string, data any) error {
	file, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(file, data); err != nil {
		return err
	}
	return nil
}
