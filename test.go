package tachyon

import (
	"bytes"
	"fmt"
)

func RunCapture(path string) ([]*Result, string, error) {
	cfg := &Config{ShowCommandOutput: false}

	ns := NewNestedScope(nil)

	env := NewEnv(ns, cfg)
	defer env.Cleanup()

	playbook, err := NewPlaybook(env, path)
	if err != nil {
		fmt.Printf("Error loading plays: %s\n", err)
		return nil, "", err
	}

	var buf bytes.Buffer

	reporter := CLIReporter{&buf}

	runner := NewRunner(env, playbook.Plays)
	runner.SetReport(&reporter)

	err = runner.Run(env)

	if err != nil {
		return nil, "", err
	}

	return runner.Results, buf.String(), nil
}
