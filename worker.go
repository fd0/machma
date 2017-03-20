package main

import (
	"bufio"
	"fmt"
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
func (c *Command) Run(outCh chan<- Status, errCh chan<- string) error {
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

	go tagLines(&wg, c.Tag, stdout, outCh)
	go tagErrors(&wg, fmt.Sprintf("[%v] error: ", c.Tag), stderr, errCh)

	defer wg.Wait()

	return cmd.Run()
}

func tagLines(wg *sync.WaitGroup, tag string, input io.Reader, out chan<- Status) {
	defer wg.Done()
	sc := bufio.NewScanner(input)
	for sc.Scan() {
		out <- Status{
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

func worker(wg *sync.WaitGroup, in <-chan *Command, outCh chan<- Status, errCh chan<- string) {
	defer wg.Done()

	for cmd := range in {
		err := cmd.Run(outCh, errCh)
		if err != nil {
			errCh <- fmt.Sprintf("%v failed: %v\n", cmd.Tag, err)
		}

		outCh <- Status{
			Tag:  cmd.Tag,
			Done: true,
		}
	}
}
