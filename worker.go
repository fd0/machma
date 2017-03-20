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

	go tagLines(&wg, c.Tag, false, stdout, outCh)
	go tagLines(&wg, c.Tag, true, stderr, outCh)

	defer wg.Wait()

	return cmd.Run()
}

func tagLines(wg *sync.WaitGroup, tag string, isError bool, input io.Reader, out chan<- Status) {
	defer wg.Done()
	sc := bufio.NewScanner(input)
	for sc.Scan() {
		out <- Status{
			Error:   isError,
			Tag:     tag,
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
			Start: true,
		}

		err := cmd.Run(outCh)
		finalStatus := Status{
			Tag:  cmd.Tag,
			Done: true,
		}
		if err != nil {
			finalStatus.Error = true
			finalStatus.Message = err.Error()
		}
		outCh <- finalStatus
	}
}
