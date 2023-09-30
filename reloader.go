package reloader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// ErrBuildCmdNotSet is returned when the build command is not set on Application.
var ErrBuildCmdNotSet = errors.New("build command not set")

// ErrRunCmdNotSet is returned when the run command is not set on Application.
var ErrRunCmdNotSet = errors.New("run command not set")

// ErrPortNotSet is returned when the port is not set on Application.
var ErrPortNotSet = errors.New("port not set")

type Application struct {
	// BuildCmd is the command run when the application needs to be built or
	// rebuilt.
	BuildCmd []string
	// RunCmd is the command run when the application needs to be run.
	RunCmd []string

	// WatchDirs is the directories that are watched for changes.
	WatchDirs []string

	// Port is the persistent port the application is running on.
	Port string

	// TargetPort is the port the traffic is forwarded to.
	TargetPort string

	// Logger logs debug, error, and info messages from the reloader and the build/run commands.
	Logger *slog.Logger

	started   chan struct{}
	startLock sync.Mutex

	watcher watcher
}

// Started is closed when the application has started for the first time.
func (app *Application) Started() <-chan struct{} {
	app.startLock.Lock()
	defer app.startLock.Unlock()

	if app.started == nil {
		app.started = make(chan struct{})
	}

	return app.started
}

// Start starts the application and listens for changes, rebuilding and
// restarting the target application in the background. Start does not block.
func (app *Application) Start(ctx context.Context) error {
	app.startLock.Lock()
	if app.started == nil {
		app.started = make(chan struct{})
	}
	app.startLock.Unlock()

	if app.BuildCmd == nil {
		close(app.started)
		return ErrBuildCmdNotSet
	}

	if app.RunCmd == nil {
		close(app.started)
		return ErrRunCmdNotSet
	}

	if app.TargetPort == "" {
		close(app.started)
		return ErrPortNotSet
	}

	if app.Port == "" {
		app.Port = ":4050"
	}

	if app.Logger == nil {
		app.Logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}

	if app.watcher == nil {
		notifier, err := fsnotify.NewWatcher()
		if err != nil {
			return fmt.Errorf("could not create fsnotify watcher: %w", err)
		}

		app.watcher = &fsWatcher{watcher: notifier}
	}

	builder := &builder{
		buildCmd: app.BuildCmd,
		runCmd:   app.RunCmd,
		logger:   app.Logger,
	}

	// Attempt initial build, if it fails return the error since we don't want
	// to start in a bad state for now.
	err := builder.Build()
	if err != nil {
		close(app.started)
		return err
	}

	target, err := url.Parse("http://127.0.0.1" + app.TargetPort)
	if err != nil {
		return fmt.Errorf("could not parse target url: %w", err)
	}

	go func() {
		err := builder.Run()
		if err != nil {
			app.Logger.Error("could not run application", "err", err)
		}
		close(app.started)
	}()

	events := make(chan string, 10)
	go func() {
		err := app.watcher.Watch(ctx, []string{"."}, events)
		if err != nil && !errors.Is(err, context.Canceled) {
			app.Logger.Error("could not watch files", "err", err)
		}
	}()

	// wait for the application to start before handling events
	<-app.Started()

	go func() {
		proxy := httputil.NewSingleHostReverseProxy(target)
		_ = http.ListenAndServe(app.Port, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Acquire a read lock to prevent the application from being rebuilt mid-request.
			builder.WithReadLock(func() {
				if builder.errText != "" {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte("encountered an error building/running the application"))
					_, _ = w.Write([]byte(builder.errText))
					return
				}

				if !builder.Running() {
					w.WriteHeader(http.StatusInternalServerError)
					fmt.Println(builder.errText)
					_, _ = w.Write([]byte("application is not running"))
					return
				}

				proxy.ServeHTTP(w, r)
			})
		}))
	}()

	for {
		select {
		case <-ctx.Done():
			builder.Stop()
			return ctx.Err()
		case event := <-events:
			app.Logger.Debug("file changed", "file", event)
			// Clear existing queue to prevent rapid rebuilds.
			for i := 0; i < len(events); i++ {
				<-events
			}

			builder.Stop()
			if err = builder.Build(); err == nil {
				_ = builder.Run()
			}
		}
	}
}
