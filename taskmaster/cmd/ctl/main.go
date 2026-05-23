package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
)

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
		var resp map[string]any
		json.NewDecoder(conn).Decode(&resp)
		conn.Close()

		if e, ok := resp["error"].(string); ok && e != "" {
			fmt.Println("error:", e)
		} else {
			b, _ := json.MarshalIndent(resp["result"], "", "  ")
			fmt.Println(string(b))
		}
	}
}
