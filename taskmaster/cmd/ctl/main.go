package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
)

type rpcResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type statusReport struct {
	Name     string `json:"Name"`
	Status   string `json:"Status"`
	Pid      int    `json:"Pid"`
	ExitCode int    `json:"ExitCode"`
	Uptime   string `json:"Uptime"`
}

func main() {
	socket := "/tmp/taskmaster.sock"
	reader := bufio.NewReader(os.Stdin)
	id := 0

	for {
		fmt.Print("taskmaster> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			return
		}

		parts := strings.Fields(line)
		method := parts[0]
		var params any
		if len(parts) > 1 {
			params = map[string]string{"name": parts[1]}
		}

		id++
		req := map[string]any{"id": id, "method": method}
		if params != nil {
			req["params"] = params
		}

		conn, err := net.Dial("unix", socket)
		if err != nil {
			fmt.Println("dial:", err)
			continue
		}

		json.NewEncoder(conn).Encode(req)
		var resp rpcResponse
		if err := json.NewDecoder(conn).Decode(&resp); err != nil {
			conn.Close()
			fmt.Println("error:", err)
			continue
		}
		conn.Close()

		if resp.Error != "" {
			errText := resp.Error
			if strings.Contains(errText, "not found in manager registry") {
				errText = "no such process"
			}
			if method == "start" || method == "stop" || method == "restart" {
				if len(parts) > 1 {
					fmt.Printf("%s: ERROR (%s)\n", parts[1], errText)
				} else {
					fmt.Printf("ERROR (%s)\n", errText)
				}
			} else {
				fmt.Printf("ERROR (%s)\n", errText)
			}
			continue
		}

		switch method {
		case "status":
			var reports []statusReport
			if err := json.Unmarshal(resp.Result, &reports); err != nil {
				fmt.Println("error:", err)
				continue
			}
			sort.Slice(reports, func(i, j int) bool {
				return reports[i].Name < reports[j].Name
			})
			maxLen := 0
			for _, report := range reports {
				if len(report.Name) > maxLen {
					maxLen = len(report.Name)
				}
			}
			nameFormat := fmt.Sprintf("%%-%ds", maxLen+3)
			for _, report := range reports {
				state := strings.ToUpper(report.Status)
				desc := ""
				switch report.Status {
				case "running":
					if report.Pid > 0 {
						desc = fmt.Sprintf("pid %d, uptime %s", report.Pid, report.Uptime)
					} else {
						desc = fmt.Sprintf("uptime %s", report.Uptime)
					}
				case "stopped":
					if report.ExitCode != 0 {
						desc = fmt.Sprintf("exited (status %d)", report.ExitCode)
					} else {
						desc = "stopped"
					}
				case "starting":
					desc = "starting"
				case "backoff":
					desc = "backoff"
				case "fatal":
					desc = "fatal"
				}
				line := fmt.Sprintf(nameFormat+"%-10s%s", report.Name, state, desc)
				fmt.Println(line)
			}
		case "start":
			if len(parts) > 1 {
				fmt.Printf("%s: started\n", parts[1])
			}
		case "stop":
			if len(parts) > 1 {
				fmt.Printf("%s: stopped\n", parts[1])
			}
		case "restart":
			if len(parts) > 1 {
				fmt.Printf("%s: restarted\n", parts[1])
			}
		case "reload", "shutdown":
			fmt.Println("ok")
		default:
			fmt.Println("ok")
		}
	}
}
