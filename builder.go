package reloader

import (
	"bytes"
	"errors"
	"log/slog"
	"math"
	"os"
	"os/exec"
	"sync"
	"time"
)

var ErrAlreadyRunning = errors.New("already running")

type Builder interface {
	// Build builds the application and blocks until the build is complete.
	Build() error
	// Run runs the application and returns an error if the application can't
	// start. This should not block.
	Run() error
	// Stop should stop the application if it is running. Stop should block until
	// the application has stopped.
	Stop()
	// Running returns true if the application is currently running.
	Running() bool
	// ErrorText returns the error text from the last build or run so it can be
	// displayed to the user.
	ErrorText() string
	// WithLock runs the given function while holding a lock so the builder
	// won't stop when the function is running.
	WithLock(func())
}

type BasicBuilder struct {
	// Bacoff is the function used to calculate the backoff duration between
	// attempts to start the application. It is called with the number of
	// attempts so far and should return the number of milliseconds to wait
	// before the next attempt.
	Backoff func(attempts int) int

	buildCmd    []string
	runCmd      []string
	logger      *slog.Logger
	healthCheck func() bool

	running *exec.Cmd
	errText string

	mu sync.RWMutex
}

var _ Builder = (*BasicBuilder)(nil)

// NewBuilder returns a new builder that runs the given build and run commands.
func NewBuilder(buildCmd, runCmd []string, logger *slog.Logger, healthCheck func() bool) *BasicBuilder {
	return &BasicBuilder{
		Backoff: func(attempts int) int {
			return int(math.Pow(2, float64(attempts))) * 5
		},
		buildCmd:    buildCmd,
		runCmd:      runCmd,
		logger:      logger,
		healthCheck: healthCheck,
	}
}

// ErrorText returns the last error text from the build or run commands.
func (b *BasicBuilder) ErrorText() string {
	return b.errText
}

// WithLock runs the given function while holding a lock so the builder won't
// restart while the function is running. An sync.RWMutex is used under-the-hood
// so multiple functions can run at the same time.
func (b *BasicBuilder) WithLock(fn func()) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	fn()
}

// Run runs the application and returns an error if the application can't
// start. The function will not return until the application has started and
// passed the health check.
func (b *BasicBuilder) Run() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Running() {
		return ErrAlreadyRunning
	}

	cmd := exec.Command(b.runCmd[0], b.runCmd[1:]...)

	var stdout []byte
	var stderr []byte
	cmd.Stdout = bytes.NewBuffer(stdout)
	cmd.Stderr = bytes.NewBuffer(stderr)

	err := cmd.Start()
	b.logger.Debug("run", "cmd", b.runCmd, "err", err, "stdout", stdout, "stderr", stderr)
	if err != nil {
		b.errText = err.Error() + "\n\n" + string(stderr)
	} else {
		b.errText = ""
		b.running = cmd
	}

	start := time.Now()
	attempt := 0

	for {
		attempt++
		if b.healthCheck() {
			break
		} else {
			b.logger.Debug("health check failed", "attempt", attempt, "duration", time.Since(start))
		}

		if !b.Running() {
			return errors.New("application stopped")
		}

		if time.Since(start) > 10*time.Second {
			b.errText = "health check timed out"
			break
		}

		time.Sleep(
			time.Duration(b.Backoff(attempt)) * time.Millisecond,
		)
	}

	return err
}

// Build builds the application and blocks until the build is complete.
func (b *BasicBuilder) Build() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.Running() {
		return ErrAlreadyRunning
	}

	cmd := exec.Command(b.buildCmd[0], b.buildCmd[1:]...)

	var stdout []byte
	var stderr []byte
	cmd.Stdout = bytes.NewBuffer(stdout)
	cmd.Stderr = bytes.NewBuffer(stderr)

	err := cmd.Run()
	b.logger.Debug("build", "cmd", b.buildCmd, "err", err, "stdout", stdout, "stderr", stderr)
	if err != nil {
		b.errText = err.Error() + "\n\n" + string(stdout) + "\n\n" + string(stderr)
	} else {
		b.errText = ""
	}

	return err
}

// Stop should stop the application if it is running. Stop blocks until the
// application has stopped.
func (b *BasicBuilder) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.Running() {
		_ = b.running.Process.Signal(os.Interrupt)
		_ = b.running.Wait()
	}
	b.running = nil
}

// Running returns true if the application is currently running. Running should
// only be called when holding the lock.
func (b *BasicBuilder) Running() bool {
	return b.running != nil
}
