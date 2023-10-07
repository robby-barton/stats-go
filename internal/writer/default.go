package writer

import (
	"context"
	"os"
	"path/filepath"
)

type DefaultWriter struct{}

func (*DefaultWriter) WriteData(_ context.Context, fileName string, data []byte) error {
	err := os.MkdirAll(filepath.Dir(fileName), 0775)
	if err != nil {
		return err
	}

	return os.WriteFile(fileName, data, 0664) // #nosec G306
}
