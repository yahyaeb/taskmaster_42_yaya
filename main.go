package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Programs map[string]*Settings

type Status string

const (
	STARTING Status = "starting"
	RUNNING  Status = "running"
	STOPPED  Status = "stopped"
	FATAL    Status = "fatal"
)

type Settings struct {
	Name   string
	Cmd    string `yaml:"cmd"`
	Pid    int
	Status Status
}

func watchdog(settings *Settings, updates chan Settings) {
	for {
		parts := strings.Fields(settings.Cmd)

		if len(parts) == 0 {
			fmt.Printf("No command specified for program %s\n", settings.Name)
			updates <- Settings{Name: settings.Name, Status: FATAL}
			return
		}

		cmd := exec.Command(parts[0], parts[1:]...)

		if err := cmd.Start(); err != nil {
			fmt.Printf("Error starting program %s: %v\n", settings.Name, err)
			updates <- Settings{Name: settings.Name, Status: FATAL}
			time.Sleep(5 * time.Second)
			continue
		}

		updates <- Settings{Name: settings.Name, Pid: cmd.Process.Pid, Status: RUNNING}

		fmt.Printf("Started program %s with PID %d\n", settings.Name, cmd.Process.Pid)

		err := cmd.Wait()

		if err != nil {
			fmt.Printf("Program %s exited with error: %v\n", settings.Name, err)
			updates <- Settings{Name: settings.Name, Status: FATAL, Pid: 0}
		}

		fmt.Printf("Program %s exited successfully\n", settings.Name)
		updates <- Settings{Name: settings.Name, Status: STOPPED, Pid: 0}

		time.Sleep(1 * time.Second)

	}
}

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

func reloadConfig(stopChans map[string]chan struct{}, programs Programs) (Programs, error) {
	data, err := os.ReadFile("config.yml")
	if err != nil {
		return nil, err
	}

	var settings Programs
	err = yaml.Unmarshal(data, &settings)
	if err != nil {
		return nil, err
	}

	for name := range programs {
		if _, exists := settings[name]; !exists {
			stopChans[name] <- struct{}{}
			delete(settings, name)
			fmt.Printf("Stopped program %s\n", name)
		}
	}
	return settings, nil
}

func main() {
	data, err := os.ReadFile("config.yml")

	if err != nil {
		panic(err)
	}

	var settings Programs
	err = yaml.Unmarshal(data, &settings)

	if err != nil {
		panic(err)
	}

	manager := struct {
		updates chan Settings
		input   chan string
		stop    map[string]chan struct{}
		sighup  chan os.Signal
	}{
		updates: make(chan Settings),
		input:   make(chan string),
		stop:    make(map[string]chan struct{}),
		sighup:  make(chan os.Signal, 1),
	}

	signal.Notify(manager.sighup, syscall.SIGHUP)

	for name, settings := range settings {
		settings.Name = name
		go watchdog(settings, manager.updates)
	}

	go read(manager.input)

	for {
		fmt.Print("> ")

		select {
		case <-manager.sighup:
			fmt.Println("reloading configuration...")
			newSettings, err := reloadConfig(manager.stop, settings)
			if err != nil {
				fmt.Printf("Error reloading configuration: %v\n", err)
				continue
			}

			for name, setting := range newSettings {
				if _, exists := settings[name]; !exists {
					setting.Name = name
					go watchdog(setting, manager.updates)
					fmt.Printf("Started new program %s\n", name)
				}
			}

			settings = newSettings
		case msg := <-manager.updates:
			// The Manager updates the central state
			if setting, ok := settings[msg.Name]; ok {
				setting.Status = msg.Status
				setting.Pid = msg.Pid
			}
		case input := <-manager.input:
			switch input {
			case "exit":
				for _, setting := range settings {
					if setting.Pid > 0 {
						process, _ := os.FindProcess(setting.Pid)
						process.Kill()
					}
				}
			case "status":
				for name, setting := range settings {
					fmt.Printf("Program: %s | Status: %s | PID: %d\n", name, setting.Status, setting.Pid)
				}
			case "reload":
				fmt.Println("reloading configuration...")
				self, _ := os.FindProcess(os.Getpid())
				self.Signal(syscall.SIGHUP)
			default:
				fmt.Println("Unknown command:", input)
			}
		}
	}
}
