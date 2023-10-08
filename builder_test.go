package reloader

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuilder_HealthCheck(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	healthCheckValue := false
	builder := NewBuilder(
		[]string{"go", "build", "-o", "./testserver", "./internal/testserver"},
		[]string{"./testserver"},
		logger,
		func() bool { return healthCheckValue },
	)
	builder.Backoff = func(attempts int) int {
		return 1
	}
	defer builder.Stop()

	err := builder.Build()
	require.NoError(t, err)

	done := false
	go func() {
		defer func() { done = true }()
		err := builder.Run()
		require.NoError(t, err)
	}()

	require.False(t, done)
	time.Sleep(10 * time.Millisecond)
	require.False(t, done)

	healthCheckValue = true
	time.Sleep(10 * time.Millisecond)
	require.True(t, done)
}
