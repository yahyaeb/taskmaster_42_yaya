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

type Status string

type Autostart string

const (
	STARTING Status = "starting"
	RUNNING  Status = "running"
	STOPPED  Status = "stopped"
	FATAL    Status = "fatal"
)

const (
	ALWAYS     = "always"
	NEVER      = "never"
	UNEXPECTED = "unexpected"
)

type Update struct {
	Name   string
	Status Process
}

type Process struct {
	Stat      Status
	Pid       int
	retries   int
	lastStart time.Time
	intended  bool
}

type Manager struct {
	Config  map[string]*Settings
	Process map[string]*Settings
}

type Settings struct {
	ProcessName   string            `yaml:"process_name"`
	Program       string            `yaml:"program"`
	Cmd           string            `yaml:"cmd"`
	Numprocs      int               `yaml:"numprocs"`
	NumprocsStart int               `yaml:"numprocs_start"`
	Umask         int               `yaml:"umask"`
	Workingdir    string            `yaml:"workingdir"`
	Autostart     bool              `yaml:"autostart"`
	Autorestart   Autostart         `yaml:"autorestart"`
	Exitcodes     []int             `yaml:"exitcodes"`
	Startretries  int               `yaml:"startretries"`
	Starttime     int               `yaml:"starttime"`
	Stopsignal    string            `yaml:"stopsignal"`
	Stoptime      int               `yaml:"stoptime"`
	Stdout        string            `yaml:"stdout"`
	Stderr        string            `yaml:"stderr"`
	Env           map[string]string `yaml:"env"`
	Status        Process
}

func formatProcessName(name string, num int) string {
	return fmt.Sprintf("%s:%02d", name, num)
}
func stop(manager *Manager, stops map[string]chan struct{}) {

	for name, ch := range stops {
		if ch != nil {
			safe(ch)
			delete(stops, name)
			fmt.Printf("Stopped program %s\n", name)
		}
	}

	max := 0
	for _, mgr := range manager.Process {
		if mgr.Stoptime > max {
			max = mgr.Stoptime
		}
	}

	timeout := time.Now().Add(time.Duration(max+3) * time.Second)
	for time.Now().Before(timeout) {
		stopped := true
		for _, mgr := range manager.Process {
			if mgr.Status.Pid > 0 && mgr.Status.Stat == RUNNING {
				stopped = false
				break
			}
		}
		if stopped {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	for _, mgr := range manager.Process {
		if mgr.Status.Pid > 0 {
			proc, _ := os.FindProcess(mgr.Status.Pid)
			_ = proc.Kill()
		}
	}
}

func openAndAttachFiles(cmd *exec.Cmd, setting *Settings) (outF, errF *os.File, err error) {
	if setting.Stdout != "" {
		outF, err = os.OpenFile(setting.Stdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("open stdout %s: %w", setting.Stdout, err)
		}
		cmd.Stdout = outF
	}
	if setting.Stderr != "" {
		errF, err = os.OpenFile(setting.Stderr, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			if outF != nil {
				outF.Close()
			}
			return nil, nil, fmt.Errorf("open stderr %s: %w", setting.Stderr, err)
		}
		cmd.Stderr = errF
	} else if outF != nil {
		cmd.Stderr = outF
		errF = outF
	}
	return outF, errF, nil
}

func watchdog(settings *Settings, updates chan Update, stop chan struct{}) {

	parts := strings.Fields(settings.Cmd)

	if len(parts) == 0 {
		fmt.Printf("No command specified for program %s\n", settings.Program)
		updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL}}
		return
	}

	// Autostart check
	if !settings.Autostart {
		fmt.Printf("Program %s is set to not autostart, skipping...\n", settings.Program)
		updates <- Update{Name: settings.ProcessName, Status: Process{Stat: STOPPED}}
		return
	}

	// prepare command
	cmd := exec.Command(parts[0], parts[1:]...)

	if settings.Workingdir != "" {
		cmd.Dir = settings.Workingdir
	}

	env := os.Environ()
	if settings.Env != nil {
		for key, value := range settings.Env {
			env = append(env, fmt.Sprintf("%s=%s", key, value))
		}
		fmt.Printf("Set environment for program %s: %v\n", settings.Program, settings.Env)
	}
	cmd.Env = env

	outF, errF, err := openAndAttachFiles(cmd, settings)
	if err != nil {
		fmt.Printf("Error setting up output files for program %s: %v\n", settings.Program, err)
		updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL}}
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Error starting program %s: %v\n", settings.Program, err)
		updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL}}
		time.Sleep(5 * time.Second)
		return
	}

	// notify RUNNING
	updates <- Update{Name: settings.ProcessName, Status: Process{Stat: RUNNING, Pid: cmd.Process.Pid}}
	fmt.Printf("Started program %s with PID %d\n", settings.Program, cmd.Process.Pid)

	// single waiter that publishes result to a done/broadcast
	done := make(chan error, 1)
	go func(c *exec.Cmd, of, ef *os.File) {
		err := c.Wait()
		if of != nil {
			of.Close()
		}
		if ef != nil && ef != of {
			ef.Close()
		}
		done <- err
	}(cmd, outF, errF)

	// broadcast the single wait result
	finished := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = <-done
		close(finished)
	}()

	// wait for either stop request or process finish
	select {
	case <-stop:
		// request graceful stop
		if cmd.Process != nil {
			_ = cmd.Process.Signal(syscall.SIGTERM)
		}
		// wait for finish or force kill after Stoptimeout
		waitTimeout := time.Duration(settings.Stoptime) * time.Second
		if settings.Stoptime == 0 {
			waitTimeout = 5 * time.Second
		}
		select {
		case <-finished:
			if waitErr != nil {
				fmt.Printf("Program %s exited with error after stop: %v\n", settings.Program, waitErr)
				updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL, Pid: 0}}
			} else {
				fmt.Printf("Program %s stopped gracefully\n", settings.Program)
				updates <- Update{Name: settings.ProcessName, Status: Process{Stat: STOPPED, Pid: 0}}
			}
		case <-time.After(waitTimeout):
			if cmd.Process != nil {
				// escalate to SIGKILL if SIGTERM did not stop the child
				err := cmd.Process.Signal(syscall.SIGKILL)
				if err != nil {
					fmt.Printf("Failed to SIGKILL program %s (pid %d): %v\n", settings.Program, cmd.Process.Pid, err)
				} else {
					fmt.Printf("Sent SIGKILL to program %s (pid %d)\n", settings.Program, cmd.Process.Pid)
				}
			}
			<-finished
			if waitErr != nil {
				fmt.Printf("Program %s killed after timeout, error: %v\n", settings.Program, waitErr)
				updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL, Pid: 0}}
			} else {
				fmt.Printf("Program %s killed after timeout\n", settings.Program)
				updates <- Update{Name: settings.ProcessName, Status: Process{Stat: STOPPED, Pid: 0}}
			}
		}
		return
	case <-finished:
		// process exited on its own
		if waitErr != nil {
			fmt.Printf("Program %s exited with error: %v\n", settings.Program, waitErr)
			updates <- Update{Name: settings.ProcessName, Status: Process{Stat: FATAL, Pid: 0}}
		} else {
			fmt.Printf("Program %s exited successfully\n", settings.Program)
			updates <- Update{Name: settings.ProcessName, Status: Process{Stat: STOPPED, Pid: 0}}
		}
		return
	}
}

func read(inputChan chan string) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		inputChan <- scanner.Text()
	}
}

func spawn(prev *Manager, curr *Manager, updates chan Update, stops map[string]chan struct{}) {
	for name, setting := range curr.Process {
		if _, exists := prev.Process[name]; !exists {
			s := *setting

			if _, ok := stops[name]; !ok {
				stops[name] = make(chan struct{})
			}

			go watchdog(&s, updates, stops[name])
			fmt.Printf("Started new program %s\n", setting.ProcessName)
		}
	}

	for name := range prev.Process {
		if _, exists := curr.Process[name]; !exists {
			if ch, ok := stops[name]; ok {
				close(ch)
				delete(stops, name)
			}
			fmt.Printf("Stopped removed program %s\n", name)
		}
	}
}

func load(manager *Manager) (*Manager, error) {

	data, err := os.ReadFile("config.yml")

	if err != nil {
		panic(err)
	}

	// parse raw settings then wrap into Managers
	var raw map[string]*Settings
	err = yaml.Unmarshal(data, &raw)
	if err != nil {
		panic(err)
	}

	process := make(map[string]*Settings)
	for name, s := range raw {
		// build a map of instance-name -> settings for this program
		for i := 0; i < s.Numprocs; i++ {
			ns := *s
			instName := formatProcessName(name, i)
			ns.ProcessName = instName
			process[instName] = &ns
		}
	}

	manager = &Manager{Config: raw, Process: process}

	return manager, nil
}

func safe(ch chan struct{}) {
	defer func() { recover() }()
	close(ch)
}

func main() {

	ctl := struct {
		updates chan Update
		input   chan string
		stop    map[string]chan struct{}
		sighup  chan os.Signal
	}{
		updates: make(chan Update),
		input:   make(chan string),
		stop:    make(map[string]chan struct{}),
		sighup:  make(chan os.Signal, 1),
	}

	manager, err := load(&Manager{})

	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	spawn(&Manager{}, manager, ctl.updates, ctl.stop)
	signal.Notify(ctl.sighup, syscall.SIGHUP)

	go read(ctl.input)

	for {
		fmt.Print("> ")

		select {
		case <-ctl.sighup:
			fmt.Println("Hot-reloading configuration...")

			newManager, err := load(&Manager{})

			if err != nil {
				fmt.Printf("Error reloading configuration: %v\n", err)
				continue
			}

			spawn(manager, newManager, ctl.updates, ctl.stop)

			manager = newManager

		case msg := <-ctl.updates:
			// The Manager updates the central state
			if mgr, ok := manager.Process[msg.Name]; ok {
				mgr.Status = msg.Status
				fmt.Printf("Updated status for program %s: %s\n", msg.Name, msg.Status.Stat)
			}
		case input := <-ctl.input:
			switch input {
			case "exit":
				stop(manager, ctl.stop)
			case "status":
				for name, mgr := range manager.Process {
					fmt.Printf("Program: %s | Status: %s | PID: %d\n", name, mgr.Status.Stat, mgr.Status.Pid)
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
