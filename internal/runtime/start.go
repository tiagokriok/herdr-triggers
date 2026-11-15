package runtime

import (
	"bufio"
	"io"
	"os/exec"
	"syscall"
)

func Start(argv []string) (<-chan string, func(), error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.SysProcAttr = sysProcAttr()
	r, w := io.Pipe()
	cmd.Stdout, cmd.Stderr = w, w
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	lines := make(chan string)
	go func() {
		cmd.Wait()
		w.Close()
	}()
	go func() {
		defer close(lines)
		s := bufio.NewScanner(r)
		for s.Scan() {
			lines <- s.Text()
		}
	}()
	return lines, func() { cmd.Process.Signal(syscall.SIGTERM) }, nil
}
