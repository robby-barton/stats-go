package writer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
)

type DefaultWriter struct{}

func (*DefaultWriter) WriteData(_ context.Context, fileName string, input any) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0775)
	if err != nil {
		return err
	}

	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	return os.WriteFile(fileName, data, 0664) // #nosec G306
}
