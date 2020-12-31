package tproxy

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func Setup(cmd, ip string, port, portMin, portMax uint32) (out []byte, err error) {
	if cmd == "iptables" {
		return run("iptables", strings.Split(fmt.Sprintf(
			"-t mangle -I PREROUTING -d %s -p tcp --dport %d:%d -j TPROXY --on-port=%d", ip, portMin, portMax, port,
		), " ")...)
	}

	out, err = run("nft", "add table ip zenvoy")
	if err == nil {
		out, err = run("nft", `add chain ip zenvoy proxy { type filter hook prerouting priority 0 ; }`)
	}
	if err == nil {
		out, err = run("nft", fmt.Sprintf("add rule ip zenvoy proxy ip daddr %s tcp dport %d-%d tproxy to :%d", ip, portMin, portMax, port))
	}
	return
}

func run(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}
