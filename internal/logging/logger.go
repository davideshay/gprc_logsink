package logging

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
)

const LevelTrace = slog.Level(-8)

// Setup configures and returns a structured logger
func Setup() *slog.Logger {
	level := getLogLevelFromEnv("LOG_LEVEL")

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level:     level,
		AddSource: level == slog.LevelDebug, // Add source info for debug
	})

	logger := slog.New(handler)

	// Log startup info
	logger.Info("Logger initialized",
		slog.String("level", level.Level().String()),
		slog.String("go_version", strings.TrimPrefix(runtime.Version(), "go")))

	logger.Info("Sample messages logged for this level:")
	logger.Error("Error message")
	logger.Warn("Warning MEssage")
	logger.Info("Info message")
	logger.Debug("Debug message")
	logger.Log(context.Background(), LevelTrace, "Trace Message")

	return logger
}

// getLogLevelFromEnv parses log level from environment variable
func getLogLevelFromEnv(envVar string) slog.Leveler {
	levelStr := strings.ToLower(os.Getenv(envVar))
	switch levelStr {
	case "trace", "t", "0":
		return LevelTrace
	case "debug", "d", "1":
		return slog.LevelDebug
	case "info", "information", "i", "2":
		return slog.LevelInfo
	case "warn", "warning", "w", "3":
		return slog.LevelWarn
	case "error", "err", "e", "4":
		return slog.LevelError
	default:
		return slog.LevelInfo // default level
	}
}
