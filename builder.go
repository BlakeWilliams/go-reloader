package reloader

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
)

var ErrAlreadyRunning = errors.New("already running")

type builder struct {
	buildCmd []string
	runCmd   []string
	logger   *slog.Logger

	running *exec.Cmd
	errText string

	mu sync.RWMutex
}

func (b *builder) WithReadLock(fn func()) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	fn()
}

func (b *builder) Run() error {
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
		fmt.Println("state", cmd.ProcessState)
		b.running = cmd
	}

	fmt.Println("run")
	return err
}

func (b *builder) Build() error {
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
		b.errText = err.Error() + "\n\n" + string(stderr)
	} else {
		b.errText = ""
	}

	return err
}

func (b *builder) Stop() {
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
func (b *builder) Running() bool {
	return b.running != nil
}
