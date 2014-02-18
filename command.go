package tachyon

import (
  "sync"
)

type Command interface {
  Run(env *Environment, pe *PlayEnv, args string) error
}

type Commands map[string]Command

var AvailableCommands Commands

var initAvailable sync.Once

func RegisterCommand(name string, cmd Command) {
  initAvailable.Do(func() {
    AvailableCommands = make(Commands)
  })

  AvailableCommands[name] = cmd
}

