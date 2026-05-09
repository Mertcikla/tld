package watch

import (
	"context"
	"time"
)

type EventLogger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

func logInfo(ctx context.Context, logger EventLogger, msg string, args ...any) {
	if logger != nil {
		logger.InfoContext(ctx, msg, args...)
	}
}

func logError(ctx context.Context, logger EventLogger, msg string, err error, args ...any) {
	if logger == nil {
		return
	}
	fields := append([]any{"error", err}, args...)
	logger.ErrorContext(ctx, msg, fields...)
}

func logElapsed(started time.Time) string {
	return time.Since(started).Round(time.Millisecond).String()
}
