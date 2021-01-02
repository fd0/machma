package main

import (
	"bufio"
	"context"
	"io"
	"os/exec"
	"sync"
)

// Command collects all information of one invocation of a command.
type Command struct {
	Cmd  string
	Args []string

	ID  int
	Tag string
}

// Run executes the command.
func (c *Command) Run(ctx context.Context, outCh chan<- Status) error {
	cmd := exec.Command(c.Cmd, c.Args...) //nolint:gosec

	// make sure the new process and all children get a new process group ID
	createProcessGroup(cmd)

	// done is closed when the process has exited
	done := make(chan struct{})

	// wg tracks all goroutines started
	var wg sync.WaitGroup

	// start a goroutine which kills the process group when the context is cancelled
	wg.Add(1)

	go func() {
		select {
		case <-ctx.Done():
			_ = killProcessGroup(cmd)
		case <-done:
		}
		wg.Done()
	}()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	wg.Add(1)
	go c.tagLines(&wg, false, stdout, outCh) //nolint:wsl

	wg.Add(1)
	go c.tagLines(&wg, true, stderr, outCh) //nolint:wsl

	err = cmd.Run()

	close(done)
	wg.Wait()

	return err
}

func (c *Command) tagLines(wg *sync.WaitGroup, isError bool, input io.Reader, out chan<- Status) {
	defer wg.Done()

	sc := bufio.NewScanner(input)
	for sc.Scan() {
		out <- Status{
			Error:   isError,
			Tag:     c.Tag,
			ID:      c.ID,
			Message: sc.Text(),
		}
	}
}

func worker(wg *sync.WaitGroup, in <-chan *Command, outCh chan<- Status) {
	defer wg.Done()

	for cmd := range in {
		outCh <- Status{
			Tag:   cmd.Tag,
			ID:    cmd.ID,
			Start: true,
		}

		ctx := context.Background()

		var cancel context.CancelFunc

		if opts.workerTimeout > 0 {
			ctx, cancel = context.WithTimeout(context.Background(), opts.workerTimeout)
		}

		err := cmd.Run(ctx, outCh)
		finalStatus := Status{
			Tag:  cmd.Tag,
			ID:   cmd.ID,
			Done: true,
		}

		if err != nil {
			finalStatus.Error = true
			finalStatus.Message = err.Error()
		}

		outCh <- finalStatus

		if cancel != nil {
			cancel()
		}
	}
}
