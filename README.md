[![Status badge for tests](https://github.com/fd0/machma/workflows/Build%20and%20tests/badge.svg)](https://github.com/fd0/machma/actions?query=workflow%3A%22Build+and+tests%22)

# machma - Easy parallel execution of commands with live feedback

## Introduction

In order to fully utilize modern machines, jobs need to be run in parallel. For
example, resizing images sequentially takes a lot of time, whereas working on
multiple images in parallel makes much better use of a multi-core CPU and
therefore is much faster. This tool makes it very easy to execute tasks in
parallel and provides live feedback. In case of errors or lines printed by the
program, the messages are tagged with the job name.

`machma` by default reads newline-separated values and replaces all
command-line arguments set to `{}` with the file name. The number of jobs is
set to the number of cores for the CPU of the host `machma` is running on.

## Sample Usage

Resize all images found in the current directory and sub-directories to
1200x1200 pixel at most:

```shell
$ find . -iname '*.jpg' | machma --  mogrify -resize 1200x1200 -filter Lanczos {}
```

The command specified after the double dash (`--`) is executed with each
parameter that is set to `{}` replaced with the file name. At the bottom, a few
status lines are printed after a summary line. The lines below visualize the
status of the instances of the program running in parallel. The line for an
instance will either contain the name of the file (in this case) that is being
processed followed by the newest message printed by the program.

![demo: resizing files](demos/demo1.gif)


Ping a large number of hosts, but only run two jobs in parallel:

```shell
$ cat /tmp/ips | machma -p 2 -- ping -c 2 -q {}
```

The program `ping` will exit with an error code when the host is not reachable,
and `machma` prints an error message for all jobs which returned an error code.

![demo: ping hosts](demos/demo2a.gif)

A slightly more sophisticated (concerning shell magic) example is the
following, which does the same but recduces the output printed by `ping` a lot:

```shell
$ cat /tmp/ips | machma -- sh -c 'ping -c 2 -q $0 > /dev/null && echo alive' {}
```

![demo: ping hosts again](demos/demo2b.gif)


Using `--timeout` you can limit the time mogrify is allowed to run per picture. (Prevent jobs from 'locking up')
The value for timeout is formatted in golang [time.Duration format](https://golang.org/pkg/time/#Duration).
When the timeout is reached the program gets canceled.

```shell
$ find . -iname '*.jpg' | machma --timeout 5s --  mogrify -resize 1200x1200 -filter Lanczos {}
```

### Files With Spaces

Sometimes filenames have spaces, which may be problematic with shell commands.
Most of the time, this should not be a problem at all, since `machma` runs
programs directly (using the `execve` syscall on Linux for example) instead of
using `system()`. For all other cases there's the `--null` (short: `-0`) option
which instructs `machma` to read items separated by null bytes from stdin. This
can be used with the option `-print0` of the `find` command like this:

```shell
$ find . -iname '*.jpg' -print0 | machma --null --  mogrify -resize 1200x1200 -filter Lanczos {}
```

## Installation

Installation is very easy, install a recent version of Go and run:

```shell
$ go run build.go
```

Afterwards you can view the online help:
```shell
$ ./machma --help
Usage of ./machma:
      --no-id              hide the job id in the log
      --no-name            hide the job name in the log
      --no-timestamp       hide the time stamp in the log
  -0, --null               use null bytes as input separator
  -p, --procs int          number of parallel programs (default 2)
      --replace string     replace this string in the command to run (default "{}")
      --timeout duration   set maximum runtime per queued job (0s == no limit)
```
