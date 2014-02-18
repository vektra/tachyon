package main

import (
  "fmt"
  "github.com/vektra/tachyon"
  "os"
)

func main() {
  if len(os.Args) != 2 {
    fmt.Printf("Usage: tachyon <playbook>\n")
    os.Exit(1)
  }

  playbook, err := tachyon.LoadPlaybook(os.Args[1])
  
  env := &tachyon.Environment{}

  err = playbook.Run(env)

  if err != nil {
    fmt.Printf("Error running playbook: %s\n", err)
    os.Exit(1)
  }
}
