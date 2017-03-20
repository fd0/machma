package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"

	"github.com/fd0/termstatus"
	"github.com/spf13/pflag"
)

var opts = struct {
	threads     int
	placeholder string
}{}

func init() {
	pflag.IntVarP(&opts.threads, "procs", "p", runtime.NumCPU(), "number of parallel porgrams")
	pflag.StringVar(&opts.placeholder, "replace", "{}", "replace this string in the command to run")
	pflag.Parse()
}

func parseInput(ch chan<- *Command, cmd string, args []string) {
	defer close(ch)

	sc := bufio.NewScanner(os.Stdin)

	for sc.Scan() {
		cmdName := cmd
		cmdArgs := make([]string, len(args))

		line := sc.Text()

		if cmd == opts.placeholder {
			cmdName = line
		}

		for i, arg := range args {
			if arg == opts.placeholder {
				cmdArgs[i] = line
				continue
			}

			cmdArgs[i] = args[i]
		}

		ch <- &Command{
			Tag:  line,
			Cmd:  cmdName,
			Args: cmdArgs,
		}
	}
}

func checkForPlaceholder(cmdname string, args []string) {
	if cmdname == opts.placeholder {
		return
	}

	for _, arg := range args {
		if arg == opts.placeholder {
			return
		}
	}

	fmt.Fprintf(os.Stderr, "no placeholder found\n")
	os.Exit(2)
}

// Status is one message printed by a command.
type Status struct {
	Tag     string
	Message string
	Done    bool
	Error   bool
}

func updateTerminal(t *termstatus.Terminal, data map[string]string) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))

	lines := make([]string, 0, len(data)+3)
	lines = append(lines, "\n")
	lines = append(lines, fmt.Sprintf("Last output of %d workers:", opts.threads))
	for _, key := range keys {
		lines = append(lines, data[key])
	}

	t.SetStatus(lines)
}

func status(ctx context.Context, t *termstatus.Terminal, output <-chan Status, error <-chan string) {
	stat := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			return
		case err := <-error:
			t.Print(err)
		case s := <-output:
			if s.Done {
				delete(stat, s.Tag)
				continue
			}
			t.Printf("%v %v", s.Tag, s.Message)

			stat[s.Tag] = fmt.Sprintf("%v %v", s.Tag, s.Message)

			updateTerminal(t, stat)
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t := termstatus.New(ctx, os.Stdout)
	outCh := make(chan Status)
	errCh := make(chan string)

	go status(ctx, t, outCh, errCh)

	ch := make(chan *Command)

	var wg sync.WaitGroup
	for i := 0; i < opts.threads; i++ {
		wg.Add(1)
		go worker(&wg, ch, outCh, errCh)
	}

	args := pflag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "no command given\n")
		pflag.Usage()
		os.Exit(1)
	}

	cmdname := args[0]
	args = args[1:]

	checkForPlaceholder(cmdname, args)

	go parseInput(ch, cmdname, args)

	wg.Wait()

	t.Finish()
}
