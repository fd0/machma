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
	workerTimeout    time.Duration
	useNullSeparator bool
	hideJobID        bool
	hideTimestamp    bool
	hideName         bool
}{}

func init() {
	pflag.IntVarP(&opts.threads, "procs", "p", runtime.NumCPU(), "number of parallel programs")
	pflag.StringVar(&opts.placeholder, "replace", "{}", "replace this string in the command to run")
	pflag.DurationVar(&opts.workerTimeout, "timeout", 0*time.Second, "set maximum runtime per queued job (0s == no limit)")
	pflag.BoolVarP(&opts.useNullSeparator, "null", "0", false, "use null bytes as input separator")
	pflag.BoolVar(&opts.hideJobID, "no-id", false, "hide the job id in the log")
	pflag.BoolVar(&opts.hideTimestamp, "no-timestamp", false, "hide the time stamp in the log")
	pflag.BoolVar(&opts.hideName, "no-name", false, "hide the job name in the log")
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

func parseInput(ch chan<- *Command, jobNumCh chan<- int, cmd string, args []string) {
	defer close(ch)

	sc := bufio.NewScanner(os.Stdin)

	if opts.useNullSeparator {
		sc.Split(ScanNullSeparatedValues)
	}

	jobnum := 0
	defer func() {
		jobNumCh <- jobnum
		close(jobNumCh)
	}()

	for sc.Scan() {
		jobnum++

		cmdName := cmd
		cmdArgs := make([]string, 0, len(args))

		line := strings.TrimSpace(sc.Text())

		if line == "" {
			fmt.Fprintf(os.Stderr, "ignoring empty item\n")
			continue
		}

		cmdName = strings.Replace(cmd, opts.placeholder, line, -1)

		for _, arg := range args {
			cmdArgs = append(cmdArgs, strings.Replace(arg, opts.placeholder, line, -1))
		}

		ch <- &Command{
			ID:   jobnum,
			Tag:  line,
			Cmd:  cmdName,
			Args: cmdArgs,
		}

		if jobnum%10 == 0 {
			jobNumCh <- jobnum
		}
	}
}

func checkForPlaceholder(cmdname string, args []string) {
	if cmdname == opts.placeholder {
		return
	}

	for _, arg := range args {
		if strings.Contains(arg, opts.placeholder) {
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

var (
	lastLineCount          = 0
	lastLineCountReduction time.Time
	smoothLines            = 0
	etaEWMA                *ewma
	lastETA                time.Duration
	lastETAUpdate          time.Time
)

func updateTerminal(t *termstatus.Terminal, stats Stats, data map[string]string) {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var (
		status string
	)

	if stats.jobsFinal {
		if etaEWMA == nil {
			etaEWMA = newEWMA(stats.start, stats.jobs)
		}

		etaEWMA.Report(stats.processed)

		if time.Since(lastETAUpdate) > time.Second {
			lastETA = etaEWMA.ETA()
			lastETAUpdate = time.Now()

		}

		eta := "---"
		if lastETA > 0 {
			eta = formatDuration(lastETA)
		}

		status = fmt.Sprintf("[%s] %d/%d processed (%d failed) ETA %v, %d/%d workers:",
			formatDuration(time.Since(stats.start)),
			stats.processed,
			stats.jobs,
			stats.failed,
			eta,
			len(data),
			opts.threads)
	} else {
		status = fmt.Sprintf("[%s] %d/%d+ processed (%d failed), %d/%d workers:",
			formatDuration(time.Since(stats.start)),
			stats.processed,
			stats.jobs,
			stats.failed,
			len(data),
			opts.threads)
	}

	lines := make([]string, 0, len(data)+3)
	lines = append(lines, colorStatusLine(status))

	for _, key := range keys {
		lines = append(lines, data[key])
	}

	lineCount := len(lines)

	if lineCount > smoothLines {
		smoothLines = lineCount
	} else if lineCount < lastLineCount {
		lastLineCountReduction = time.Now()
	}

	if time.Since(lastLineCountReduction) > 500*time.Millisecond {
		smoothLines = lineCount
	}

	for i := 0; i < smoothLines-lineCount; i++ {
		lines = append(lines, "")
	}

	lastLineCount = lineCount

	t.SetStatus(lines)
}

const timeFormat = "2006-01-02 15:04:05"

// Stats contains information about jobs.
type Stats struct {
	start time.Time

	jobs      int
	jobsFinal bool

	processed int
	failed    int
}

func status(ctx context.Context, wg *sync.WaitGroup, t *termstatus.Terminal, outCh <-chan Status, inCount <-chan int) {
	defer wg.Done()
	data := make(map[string]string)

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	stats := Stats{
		start: time.Now(),
	}

	defer func() {
		t.Finish()
		fmt.Fprintf(color.Output, "\nprocessed %d items (%d failures) in %s\n",
			stats.processed,
			stats.failed,
			formatDuration(time.Since(stats.start)))
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
				m := ""
				if !opts.hideJobID {
					m += colorNumber(s.ID) + " "
				}

				if !opts.hideTimestamp {
					m += colorTimestamp(time.Now().Format(timeFormat)) + " "
				}

				if !opts.hideName {
					m += colorTag(s.Tag) + " "
				}

				t.Print(m + msg)
			}

			data[s.Tag] = fmt.Sprintf("%v %v", colorTag(s.Tag), msg)

			if s.Done {
				stats.processed++

				if s.Error {
					stats.failed++
				}

				delete(data, s.Tag)
			}

			updateTerminal(t, stats, data)
		case jobNum, ok := <-inCount:
			if !ok {
				stats.jobsFinal = true
				inCount = nil
				continue
			}
			stats.jobs = jobNum
			updateTerminal(t, stats, data)
		case <-ticker.C:
			updateTerminal(t, stats, data)
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
	jobNumCh := make(chan int)

	var statusWg sync.WaitGroup
	statusWg.Add(1)
	go status(ctx, &statusWg, t, outCh, jobNumCh)

	ch := make(chan *Command, 50000)

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

	go parseInput(ch, jobNumCh, cmdname, args)

	workersWg.Wait()
	close(outCh)

	statusWg.Wait()
}
