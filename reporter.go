package tachyon

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"
)

type Reporter interface {
	StartTasks(r *Runner)
	FinishTasks(r *Runner)
	StartHandlers(r *Runner)
	FinishHandlers(r *Runner)

	StartTask(task *Task, cmd Command, name, args string)
	FinishTask(task *Task, cmd Command, res *Result)

	FinishAsyncTask(act *AsyncAction)
	Progress(str string)
	JSONProgress(data []byte) error
}

type ProgressReporter interface {
	Progress(string)
	JSONProgress(data []byte) error
}

type CLIReporter struct {
	out   io.Writer
	Start time.Time
}

var sCLIReporter *CLIReporter = &CLIReporter{out: os.Stdout}

func (c *CLIReporter) StartTasks(r *Runner) {
	c.Start = r.Start
	fmt.Fprintf(c.out, "== tasks @ %v\n", r.Start)
}

func (c *CLIReporter) FinishTasks(r *Runner) {
	dur := time.Since(c.Start)

	fmt.Fprintf(c.out, "%7.3f ! Waiting on all tasks to finish...\n", dur.Seconds())
}

func (c *CLIReporter) StartHandlers(r *Runner) {
	dur := time.Since(c.Start)

	fmt.Fprintf(c.out, "%7.3f ! Running any handlers\n", dur.Seconds())
}

func (c *CLIReporter) FinishHandlers(r *Runner) {}

func (c *CLIReporter) StartTask(task *Task, cmd Command, name, args string) {
	dur := time.Since(c.Start)

	if task.Async() {
		fmt.Fprintf(c.out, "%7.3f - %s &\n", dur.Seconds(), name)
	} else {
		fmt.Fprintf(c.out, "%7.3f - %s\n", dur.Seconds(), name)
	}

	if reflect.TypeOf(cmd).Elem().NumField() == 0 {
		fmt.Fprintf(c.out, "%7.3f   %s: %s\n", dur.Seconds(), task.Command(), args)
	} else {
		fmt.Fprintf(c.out, "%7.3f   %#v\n", dur.Seconds(), cmd)
		fmt.Fprintf(c.out, "%7.3f   %s: %s\n", dur.Seconds(), task.Command(), args)
	}
}

func (c *CLIReporter) Progress(str string) {
	dur := time.Since(c.Start)

	lines := strings.Split(str, "\n")
	out := strings.Join(lines, fmt.Sprintf("\n%7.3f + ", dur.Seconds()))

	fmt.Fprintf(c.out, "%7.3f + %s\n", dur.Seconds(), out)
}

func (c *CLIReporter) JSONProgress(data []byte) error {
	cr := JsonChunkReconstitute{c}
	return cr.Input(data)
}

func (c *CLIReporter) FinishTask(task *Task, cmd Command, res *Result) {
	if res == nil {
		return
	}

	dur := time.Since(c.Start)

	indent := fmt.Sprintf("%7.3f   ", dur.Seconds())

	if render, ok := res.Get("_result"); ok {
		out, ok := render.Read().(string)
		if ok {
			out = strings.TrimSpace(out)

			if out != "" {
				lines := strings.Split(out, "\n")
				indented := strings.Join(lines, indent+"\n")

				fmt.Fprintf(c.out, "%7.3f * result:\n", dur.Seconds())
				fmt.Fprintf(c.out, "%7.3f     %s%s\n", dur.Seconds(), indent, indented)
			}

			return
		}
	}

	if sy, err := indentedYAML(res.Data, indent); err == nil {
		fmt.Fprintf(c.out, "%7.3f * result:\n", dur.Seconds())
		fmt.Fprintf(c.out, "%s", sy)
	}
}

func (c *CLIReporter) FinishAsyncTask(act *AsyncAction) {
	dur := time.Since(c.Start)

	if act.Error == nil {
		fmt.Fprintf(c.out, "%7.3f * %s (async success)\n", dur.Seconds(), act.Task.Name())
	} else {
		fmt.Fprintf(c.out, "%7.3f * %s (async error:%s)\n", dur.Seconds(), act.Task.Name(), act.Error)
	}
}

type JsonChunkReporter struct {
	out   io.Writer
	Start time.Time
}

func (c *JsonChunkReporter) send(args ...interface{}) {
	b := ijson(args...)
	fmt.Fprintf(c.out, "%d\n%s\n", len(b), string(b))
}

var sJsonChunkReporter *JsonChunkReporter = &JsonChunkReporter{out: os.Stdout}

func (c *JsonChunkReporter) StartTasks(r *Runner) {
	c.Start = r.Start
	c.send("phase", "start", "time", r.Start.String())
}

func (c *JsonChunkReporter) FinishTasks(r *Runner) {
	c.send("phase", "finish")
}

func (c *JsonChunkReporter) StartHandlers(r *Runner) {
	c.send("phase", "start_handlers")
}

func (c *JsonChunkReporter) FinishHandlers(r *Runner) {
	c.send("phase", "finish_handlers")
}

func (c *JsonChunkReporter) StartTask(task *Task, cmd Command, name, args string) {
	dur := time.Since(c.Start).Seconds()

	typ := "sync"

	if task.Async() {
		typ = "async"
	}

	c.send(
		"phase", "start_task",
		"type", typ,
		"name", name,
		"command", task.Command(),
		"args", args,
		"delta", dur)
}

func (c *JsonChunkReporter) Progress(str string) {
	dur := time.Since(c.Start).Seconds()

	c.send(
		"phase", "progress",
		"delta", dur,
		"progress", str)
}

func (c *JsonChunkReporter) JSONProgress(data []byte) error {
	dur := time.Since(c.Start).Seconds()

	c.send(
		"phase", "json_progress",
		"delta", dur,
		"progress", json.RawMessage(data))

	return nil
}

func (c *JsonChunkReporter) FinishTask(task *Task, cmd Command, res *Result) {
	if res == nil {
		return
	}

	dur := time.Since(c.Start).Seconds()

	c.send(
		"phase", "finish_task",
		"delta", dur,
		"result", res)
}

func (c *JsonChunkReporter) FinishAsyncTask(act *AsyncAction) {
	dur := time.Since(c.Start).Seconds()

	if act.Error == nil {
		c.send(
			"phase", "finish_task",
			"delta", dur,
			"result", act.Result)
	} else {
		c.send(
			"phase", "finish_task",
			"delta", dur,
			"error", act.Error)
	}
}

type JsonChunkReconstitute struct {
	report ProgressReporter
}

func (j *JsonChunkReconstitute) Input(data []byte) error {
	m := make(map[string]interface{})

	err := json.Unmarshal(data, &m)
	if err != nil {
		return err
	}

	phase, ok := m["phase"]
	if !ok {
		return fmt.Errorf("No phase specified")
	}

	switch phase {
	case "start":
		time, ok := m["time"]
		if !ok {
			time = "(unknown)"
		}

		j.report.Progress(fmt.Sprintf("remote tasks @ %s", time))
	case "start_task":
		j.report.Progress(fmt.Sprintf("- %s", m["name"]))
		j.report.Progress(fmt.Sprintf("  %s: %s", m["command"], m["args"]))
	case "finish_task":
		j.report.Progress(fmt.Sprintf("* result:"))

		res := m["result"].(map[string]interface{})

		j.report.Progress(indentedMap(res, "  "))
	}

	return nil
}
