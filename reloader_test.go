package reloader

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"log/slog"

	"github.com/stretchr/testify/require"
)

func TestReloader(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&b, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	app := Application{
		BuildCmd:   []string{"echo", "build"},
		RunCmd:     []string{"echo", "run"},
		TargetPort: ":4030",
		Logger:     logger,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := app.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()

	<-app.Started()
	cancel()
	<-done

	require.Contains(t, b.String(), "build")
	require.Contains(t, b.String(), "run")
}

type mockWatcher struct {
	events chan<- string
}

func (w *mockWatcher) Watch(ctx context.Context, dirs []string, events chan<- string) error {
	w.events = events
	return nil
}

func TestReloader_FileChange(t *testing.T) {
	var b bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&b, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	app := Application{
		BuildCmd:   []string{"echo", "build"},
		RunCmd:     []string{"echo", "run"},
		TargetPort: ":4030",
		Logger:     logger,
		watcher:    &mockWatcher{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := app.Run(ctx)
		require.ErrorIs(t, err, context.Canceled)
	}()

	<-app.Started()
	app.watcher.(*mockWatcher).events <- "test"
	// Validate that multiple events are ignored.
	app.watcher.(*mockWatcher).events <- "test"

	time.Sleep(5 * time.Millisecond)
	cancel()
	<-done

	logs := strings.Split(b.String(), "\n")
	require.Len(t, logs, 6)

	require.Contains(t, logs[0], "build")
	require.Contains(t, logs[1], "run")
	require.Contains(t, logs[2], "file changed")
	require.Contains(t, logs[3], "build")
	require.Contains(t, logs[4], "run")
}
