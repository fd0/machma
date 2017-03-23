package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fatih/color"
	"github.com/fd0/termstatus"
	"github.com/spf13/pflag"
)

var opts = struct {
	threads          int
	placeholder      string
	useNullSeparator bool
}{}

func init() {
	pflag.IntVarP(&opts.threads, "procs", "p", runtime.NumCPU(), "number of parallel porgrams")
	pflag.StringVar(&opts.placeholder, "replace", "{}", "replace this string in the command to run")
	pflag.BoolVarP(&opts.useNullSeparator, "null", "0", false, "use null bytes as input separator")
	pflag.Parse()
}

// ScanNullSeparatedValues splits data by null bytes
func ScanNullSeparatedValues(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	for i, b := range data {
		if b == 0 {
			return i + 1, data[:i], nil
		}
	}

	if atEOF {
		return len(data), data, nil
	}

	return 0, nil, nil
}

func parseInput(ch chan<- *Command, cmd string, args []string) {
	defer close(ch)

	sc := bufio.NewScanner(os.Stdin)

	if opts.useNullSeparator {
		sc.Split(ScanNullSeparatedValues)
	}

	jobnum := 1
	for sc.Scan() {
		cmdName := cmd
		cmdArgs := make([]string, len(args))

		line := strings.TrimSpace(sc.Text())

		if line == "" {
			fmt.Fprintf(os.Stderr, "ignoring empty item\n")
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
			ID:   jobnum,
			Tag:  line,
			Cmd:  cmdName,
			Args: cmdArgs,
		}
		jobnum++
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
	ID      int
	Tag     string
	Message string
	Error   bool

	Done  bool
	Start bool
}

func formatDuration(d time.Duration) string {
	sec := uint64(d / time.Second)

	hours := sec / 3600
	sec -= hours * 3600
	min := sec / 60
	sec -= min * 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, min, sec)
	}

	return fmt.Sprintf("%d:%02d", min, sec)
}

var (
	colorNumber    = color.New(color.Reset, color.FgGreen).SprintFunc()
	colorTimestamp = color.New(color.Reset, color.FgBlue).SprintFunc()
	colorTag       = color.New(color.Reset, color.FgYellow).SprintFunc()
	colorError     = color.New(color.Reset, color.FgRed, color.Bold).SprintFunc()

	colorStatusLine = color.New(color.Bold, color.ReverseVideo).SprintFunc()
)

func updateTerminal(t *termstatus.Terminal, start time.Time, processed, failed int, data map[string]string) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Sort(sort.StringSlice(keys))

	lines := make([]string, 0, len(data)+3)
	lines = append(lines, colorStatusLine(fmt.Sprintf("[%s] %d processed (%d failed), %d/%d workers:",
		formatDuration(time.Since(start)),
		processed,
		failed,
		len(data),
		opts.threads)))

	for _, key := range keys {
		lines = append(lines, data[key])
	}

	t.SetStatus(lines)
}

const timeFormat = "2006-01-02 15:04:05"

func status(ctx context.Context, wg *sync.WaitGroup, t *termstatus.Terminal, outCh <-chan Status) {
	defer wg.Done()
	stat := make(map[string]string)

	start := time.Now()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var stats = struct {
		processed int
		failed    int
	}{}

	defer func() {
		t.Finish()
		fmt.Fprintf(color.Output, "\nprocessed %d items (%d failures) in %s\n",
			stats.processed,
			stats.failed,
			formatDuration(time.Since(start)))
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case s, ok := <-outCh:
			if !ok {
				return
			}

			var msg string
			if s.Message != "" {
				msg = s.Message
				if s.Error {
					msg = fmt.Sprintf("%v %v", colorError("error"), colorError(s.Message))
				}
			}

			if msg != "" {
				t.Printf("%v %v %v %v",
					colorNumber(s.ID),
					colorTimestamp(time.Now().Format(timeFormat)),
					colorTag(s.Tag), msg)
			}

			stat[s.Tag] = fmt.Sprintf("%v %v", colorTag(s.Tag), msg)

			if s.Done {
				stats.processed++

				if s.Error {
					stats.failed++
				}

				delete(stat, s.Tag)
			}

			updateTerminal(t, start, stats.processed, stats.failed, stat)
		case <-ticker.C:
			updateTerminal(t, start, stats.processed, stats.failed, stat)
		}
	}
}

type fakeTerminal struct {
	io.Writer
	fd uintptr
}

func (t fakeTerminal) Fd() uintptr { return t.fd }

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var t *termstatus.Terminal
	if runtime.GOOS == "windows" {
		t = termstatus.New(ctx, &fakeTerminal{color.Output, os.Stdout.Fd()})
	} else {
		t = termstatus.New(ctx, os.Stdout)
	}
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
}
