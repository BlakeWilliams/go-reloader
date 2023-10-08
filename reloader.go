package reloader

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

// Application represents a running application that will be rebuilt and
// restarted as files change.
type Application struct {
	builder Builder
	watcher Watcher
	// Logger logs debug, error, and info messages from the reloader and the build/run commands.
	logger *slog.Logger

	targetAddr string
	started    chan struct{}
	startLock  sync.Mutex
}

// New creates a new Application with the given builder and watcher. The default
// target port is ":3000" but can be overridden via `WithTargetAddr`.
func New(builder Builder, watcher Watcher, logger *slog.Logger) *Application {
	app := &Application{
		builder:    builder,
		watcher:    watcher,
		targetAddr: ":3000",
		logger:     logger,
	}

	return app
}

// Started returns a channel that will be closed when the application has
// started.
func (app *Application) Started() <-chan struct{} {
	app.startLock.Lock()
	defer app.startLock.Unlock()

	if app.started == nil {
		app.started = make(chan struct{})
	}

	return app.started
}

// Start starts the application and listens for changes, rebuilding and
// restarting the target application in the background.
func (app *Application) ListenAndProxy(ctx context.Context, addr string, targetAddr string) error {
	app.startLock.Lock()
	if app.started == nil {
		app.started = make(chan struct{})
	}
	app.startLock.Unlock()

	err := app.builder.Build()
	if err != nil {
		return fmt.Errorf("could not build application: %w\n\t%s", err, app.builder.ErrorText())
	}

	target, err := url.Parse("http://127.0.0.1" + targetAddr)
	if err != nil {
		return fmt.Errorf("could not parse target url: %w", err)
	}

	err = app.builder.Run()
	if err != nil {
		app.logger.Error("could not run application", "err", err)
	}
	close(app.started)

	events := make(chan string, 10)
	go func() {
		err := app.watcher.Watch(ctx, events)
		if err != nil && !errors.Is(err, context.Canceled) {
			app.logger.Error("could not watch files", "err", err)
		}
	}()

	// wait for the application to start before handling events
	<-app.Started()

	go func() {
		proxy := httputil.NewSingleHostReverseProxy(target)
		_ = http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			app.logger.Debug("proxying request", "url", r.URL.String())

			for {
				completed := false
				// Acquire a read lock to prevent the application from being rebuilt mid-request.
				app.builder.WithLock(func() {
					if app.builder.ErrorText() != "" {
						app.logger.Info("failed to serve request", "url", r.URL.String(), "err", app.builder.ErrorText())
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte("encountered an error building/running the application<br>"))
						_, _ = w.Write([]byte(app.builder.ErrorText()))
						completed = true
						return
					}

					if !app.builder.Running() && app.builder.ErrorText() != "" {
						app.logger.Info("application not running", "url", r.URL.String(), "err", app.builder.ErrorText())
						w.WriteHeader(http.StatusInternalServerError)
						_, _ = w.Write([]byte("application is not running<br>"))
						_, _ = w.Write([]byte(app.builder.ErrorText()))
						completed = true
						return
					}

					if !app.builder.Running() && app.builder.ErrorText() == "" {
						return
					}

					proxy.ServeHTTP(w, r)
					completed = true
				})

				if completed {
					break
				}
			}
		}))
	}()

	for {
		select {
		case <-ctx.Done():
			app.builder.Stop()
			return ctx.Err()
		case event := <-events:
			fmt.Println(event)
			app.logger.Debug("file changed", "file", event)
			// Clear existing queue to prevent rapid rebuilds.
			for i := 0; i < len(events); i++ {
				<-events
			}

			app.builder.Stop()
			if err = app.builder.Build(); err == nil {
				_ = app.builder.Run()
			}
		}
	}
}
