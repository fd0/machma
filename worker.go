package main

import (
	"bufio"
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
func (c *Command) Run(outCh chan<- Status) error {
	cmd := exec.Command(c.Cmd, c.Args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go c.tagLines(&wg, false, stdout, outCh)
	go c.tagLines(&wg, true, stderr, outCh)

	defer wg.Wait()

	return cmd.Run()
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

func tagErrors(wg *sync.WaitGroup, tag string, input io.Reader, out chan<- string) {
	defer wg.Done()
	sc := bufio.NewScanner(input)
	for sc.Scan() {
		out <- sc.Text()
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

		err := cmd.Run(outCh)
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
	}
}
