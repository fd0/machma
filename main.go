package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
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

		line := strings.TrimSpace(sc.Text())

		if line == "" {
			fmt.Fprintf(os.Stderr, "ignoring empty line\n")
			continue
		}

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

func status(ctx context.Context, wg *sync.WaitGroup, t *termstatus.Terminal, outCh <-chan Status) {
	defer wg.Done()
	stat := make(map[string]string)

	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-outCh:
			if !ok {
				return
			}

			if s.Message != "" {
				msg := fmt.Sprintf("%v %v", s.Tag, s.Message)
				if s.Error {
					msg = fmt.Sprintf("%v error %v", s.Tag, s.Message)
				}
				t.Print(msg)

				stat[s.Tag] = msg
			}

			if s.Done {
				t.Printf("%v is done", s.Tag)
				delete(stat, s.Tag)
			}

			updateTerminal(t, stat)
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t := termstatus.New(ctx, os.Stdout)
	outCh := make(chan Status)

	var statusWg sync.WaitGroup
	statusWg.Add(1)
	go status(ctx, &statusWg, t, outCh)

	ch := make(chan *Command)

	var workersWg sync.WaitGroup
	for i := 0; i < opts.threads; i++ {
		workersWg.Add(1)
		go worker(&workersWg, ch, outCh)
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

	workersWg.Wait()
	close(outCh)

	statusWg.Wait()

	t.Finish()
}
