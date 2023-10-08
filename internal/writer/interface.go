package writer

import "context"

type Writer interface {
	WriteData(ctx context.Context, fileName string, data any) error
}
