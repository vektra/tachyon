package tachyon

import (
	"fmt"
	"io"
	"os"
	"reflect"
)

type Reporter interface {
	StartTasks(r *Runner)
	FinishTasks(r *Runner)
	StartHandlers(r *Runner)
	FinishHandlers(r *Runner)

	StartTask(task *Task, cmd Command, args string)
	FinishTask(task *Task, res *Result)

	FinishAsyncTask(act *AsyncAction)
}

type CLIReporter struct {
	out io.Writer
}

var sCLIReporter *CLIReporter = &CLIReporter{os.Stdout}

func (c *CLIReporter) StartTasks(r *Runner) {
	fmt.Fprintf(c.out, "== tasks\n")
}

func (c *CLIReporter) FinishTasks(r *Runner) {
	fmt.Fprintf(c.out, "== Waiting on all tasks to finish...\n")
}

func (c *CLIReporter) StartHandlers(r *Runner) {
	fmt.Fprintf(c.out, "== Running any handlers\n")
}

func (c *CLIReporter) FinishHandlers(r *Runner) {}

func (c *CLIReporter) StartTask(task *Task, cmd Command, args string) {
	if task.Async() {
		fmt.Fprintf(c.out, "- %s &\n", task.Name())
	} else {
		fmt.Fprintf(c.out, "- %s\n", task.Name())
	}

	if reflect.TypeOf(cmd).Elem().NumField() == 0 {
		fmt.Fprintf(c.out, "  - %s: %s\n", task.Command(), args)
	} else {
		fmt.Fprintf(c.out, "  - %#v\n  - %s: %s\n", cmd, task.Command(), args)
	}
}

func (c *CLIReporter) FinishTask(task *Task, res *Result) {
	if res == nil {
		return
	}

	fmt.Fprintf(c.out, "  - result:\n")

	if sy, err := indentedYAML(res.Data, "      "); err == nil {
		fmt.Fprintf(c.out, "%s", sy)
	}
}

func (c *CLIReporter) FinishAsyncTask(act *AsyncAction) {
	if act.Error == nil {
		fmt.Fprintf(c.out, "- %s (async success)\n", act.Task.Name())
	} else {
		fmt.Fprintf(c.out, "- %s (async error:%s)\n", act.Task.Name(), act.Error)
	}
}
