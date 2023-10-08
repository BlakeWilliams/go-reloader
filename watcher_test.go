package reloader

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/stretchr/testify/require"
)

func TestFsWatcher(t *testing.T) {
	tmpPath, err := os.MkdirTemp(os.TempDir(), "fs-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpPath)

	f, err := os.CreateTemp(tmpPath, "file")
	require.NoError(t, err)

	notifier, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	watcher := &fsWatcher{watcher: notifier, dirs: []string{tmpPath}}

	events := make(chan string)
	defer close(events)
	go func() {
		err = watcher.Watch(context.Background(), events)
		require.NoError(t, err)
	}()

	time.Sleep(1 * time.Millisecond)
	_, err = f.Write([]byte("hello"))
	f.Close()
	require.NoError(t, err)

	select {
	case e := <-events:
		require.Equal(t, f.Name(), e)
	case <-time.After(1 * time.Second):
		require.Fail(t, "timed out waiting for event")
	}
}

func TestFsWatcher_NewFile(t *testing.T) {
	tmpPath, err := os.MkdirTemp(os.TempDir(), "fs-test")
	require.NoError(t, err)
	defer os.RemoveAll(tmpPath)

	f, err := os.CreateTemp(tmpPath, "file")
	defer os.Remove(f.Name())
	require.NoError(t, err)

	notifier, err := fsnotify.NewWatcher()
	require.NoError(t, err)
	watcher := &fsWatcher{watcher: notifier, dirs: []string{tmpPath}}

	events := make(chan string)
	defer close(events)
	go func() {
		err = watcher.Watch(context.Background(), events)
		require.NoError(t, err)
	}()

	time.Sleep(1 * time.Millisecond)
	f2, err := os.CreateTemp(tmpPath, "file")
	defer os.Remove(f2.Name())

	select {
	case e := <-events:
		require.Equal(t, f2.Name(), e)
	case <-time.After(1 * time.Second):
		require.Fail(t, "timed out waiting for new file event")
	}

	_, err = f2.Write([]byte("hello"))
	f2.Close()
	require.NoError(t, err)

	select {
	case e := <-events:
		require.Equal(t, f2.Name(), e)
	case <-time.After(1 * time.Second):
		require.Fail(t, "timed out waiting for write on new file event")
	}

	require.NoError(t, err)
}
