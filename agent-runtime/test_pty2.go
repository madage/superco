package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/creack/pty"
)

func main() {
	cmd := exec.Command("claude",
		"--print",
		"--output-format", "stream-json",
		"--input-format", "stream-json",
		"--verbose",
	)

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		fmt.Fprintf(os.Stderr, "pty error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	fmt.Fprintf(os.Stderr, "PTY started pid=%d\n", cmd.Process.Pid)

	go func() {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			lower := strings.ToLower(line)
			if strings.Contains(lower, "allow") || strings.Contains(lower, "permission") {
				fmt.Fprintf(os.Stderr, "[PROMPT] %s\n", line)
				io.WriteString(f, "1\n")
				fmt.Fprintf(os.Stderr, "[SEND] 1\n")
			}
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "{") {
				fmt.Println(trimmed)
			} else if line != "" {
				fmt.Fprintf(os.Stderr, "[PTY] %s\n", line)
			}
		}
		fmt.Fprintf(os.Stderr, "[EOF]\n")
	}()

	time.Sleep(3 * time.Second)

	userMsg := `{"type":"user","message":{"role":"user","content":[{"type":"text","text":"write /tmp/pty_test.txt with content 'hello from pty'"}]}}`
	fmt.Fprintf(os.Stderr, "[SEND] user message\n")
	io.WriteString(f, userMsg+"\n")

	time.Sleep(20 * time.Second)
	fmt.Fprintf(os.Stderr, "[TIMEOUT]\n")
	cmd.Process.Kill()
}
