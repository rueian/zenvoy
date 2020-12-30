package tproxy

import (
	"fmt"
	"os"
	"os/exec"
)

func Setup(ip string, port, portMin, portMax uint32) ([]byte, error) {
	cmd := exec.Command(
		"iptables",
		"-t", "mangle",
		"-I", "PREROUTING",
		"-d", ip,
		"-p", "tcp",
		"--dport", fmt.Sprintf("%d:%d", portMin, portMax),
		"-j", "TPROXY", fmt.Sprintf("--on-port=%d", port))
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}
