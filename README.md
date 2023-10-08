# go-reloader

go-reloader is a simple implementation of Go live-reloading for development. It works by running a process that watches for changes in specified files and directories, and rebuilds + restarts the process when a change is detected.

Unlike other live-reload implementations, go-reloader is configured and run in your application so that custom loggers, build implementations, file watchers, and other aspects of the live-reload process can be customized.

go-reloader is still a WIP but is usable in its current state. Improvements and contributions are welcome!

## Usage

The example below shows how to use go-reloader to watch for changes in the `internal`, `pkg`, and `assets` directories, and restart the application when a change is detected. It uses a simple HTTP health check to determine when the application is ready to receive requests to prevent the proxy from sending requests to the application before it's ready.

```go
l := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
	Level: slog.LevelDebug,
}))

watcher, err := reloader.NewWatcher(
	"internal",
	"pkg",
	"assets",
)

if err != nil {
	panic(err)
}

appReloader := reloader.New(
	reloader.NewBuilder(
		[]string{"go", "build", "-o", "buzzword", "./cmd/app"},
		[]string{"./buzzword", "serve"},
		l,
		func() bool {
			res, err := http.Get("http://localhost:8080/_status")
			if err != nil {
				return false
			}

			if res.StatusCode != 200 {
				return false
			}

			return true
		},
	),
	watcher,
	l,
)

go func() {
	err := appReloader.ListenAndProxy(ctx, ":9000", ":8080")
	if err != nil && err != context.Canceled {
		panic(err)
	}
}()
```
