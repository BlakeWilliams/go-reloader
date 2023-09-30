package reloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

type watcher interface {
	// Watch starts watching for changes to the application.
	Watch(ctx context.Context, dirs []string, events chan<- string) error
}

type fsWatcher struct {
	watcher *fsnotify.Watcher
}

var _ watcher = (*fsWatcher)(nil)

func (w *fsWatcher) Watch(ctx context.Context, dirs []string, events chan<- string) error {
	for _, dir := range dirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return w.watcher.Add(path)
			}

			return nil
		})

		if err != nil {
			return fmt.Errorf("could not walk directory %s: %w", dir, err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-w.watcher.Events:
			switch event.Op {
			case fsnotify.Write:
				events <- event.Name
			case fsnotify.Create:
				_ = w.watcher.Add("./" + event.Name)
				events <- event.Name
			default:
				continue
			}
		case err := <-w.watcher.Errors:
			return err
		}
	}
}
