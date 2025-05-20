package cg

import (
	"encoding/json"

	"github.com/amadeusitgroup/cds/internal/cos"
)

func UnmarshalJSON(path string, data any) error {
	file, err := cos.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(file, data); err != nil {
		return err
	}
	return nil
}
