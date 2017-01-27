package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"sync"
)

type result struct {
	cmd     string
	path    string
	success bool
	pid     int
}

type Command struct {
	Profile string   `json:"profile"`
	Cmd     string   `json:"cmd"`
	Args    []string `json:"args"`
	Log     string   `json:"log"`
}

type Commands []Command

var (
	author      = "klashxx@gmail.com"
	execFile    = flag.String("exec", "", "cmd JSON file. [mandatory]")
	numRoutines = flag.Int("routines", 5, "max parallel execution routines")
)

func deserializeJSON(execFile string) (c Commands, err error) {
	rawJSON, err := ioutil.ReadFile(execFile)
	if err != nil {
		return c, err
	}

	err = json.Unmarshal(rawJSON, &c)
	if err != nil {
		return c, err
	}

	return c, nil
}

func dispatchCommands(done <-chan struct{}, c Commands) (<-chan Command, <-chan error) {
	commands := make(chan Command)
	errc := make(chan error, 1)

	go func() {
		defer close(commands)

		errc <- func() error {
			for _, p := range c {
				select {
				case commands <- p:
				case <-done:
					return errors.New("dispatch canceled")
				}
			}
			return nil
		}()
	}()

	return commands, errc
}

func commandLauncher(done <-chan struct{}, commands <-chan Command, results chan<- result) {
	var pid int

	for command := range commands {
		path, err := exec.LookPath(command.Cmd)
		if err != nil {
			log.Println("Error -> Command:", command.Cmd, "Args:", command.Args, "Error:", err)
			return
		}

		cmd := exec.Command(command.Cmd, command.Args...)
		err = cmd.Start()
		if err != nil {
			log.Println("Error -> Command:", command.Cmd, "Args:", command.Args, "Error:", err)
			return
		}

		pid = cmd.Process.Pid
		log.Println("Start -> PID:", pid, "Command:", command.Cmd, "Args:", command.Args)

		cmd.Wait()
		log.Println("End   -> PID:", pid, "Command:", command.Cmd, "Args:", command.Args)

		select {
		case results <- result{command.Cmd, path, cmd.ProcessState.Success(), pid}:
		case <-done:
			return
		}
	}
}

func main() {
	flag.Parse()
	if *execFile == "" {
		flag.PrintDefaults()
		os.Exit(5)
	}

	c, err := deserializeJSON(*execFile)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan struct{})
	defer close(done)

	commands, errc := dispatchCommands(done, c)

	results := make(chan result)

	var wg sync.WaitGroup
	wg.Add(*numRoutines)
	for i := 0; i < *numRoutines; i++ {
		go func() {
			commandLauncher(done, commands, results)
			wg.Done()
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		log.Println(r)
	}

	if err := <-errc; err != nil {
		log.Println(err)
	}
}
