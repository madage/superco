package main

import (
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/creack/pty"
)

func main() {
	// WITHOUT --print — interactive mode in PTY
	cmd := exec.Command("claude", "--verbose")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		log.Fatalf("pty: %v", err)
	}

	// Read PTY output
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				fmt.Printf("[OUT] %q\n", string(buf[:n]))
			}
		}
		fmt.Println("=== reader done ===")
	}()

	time.Sleep(3 * time.Second)

	// Write input
	fmt.Fprintln(f, "write a file called hello.txt with content hello world")

	time.Sleep(15 * time.Second)
	fmt.Println("=== cleaning up ===")
	f.Close()
	cmd.Wait()
	fmt.Println("=== done ===")
}
