package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/mritd/sshutils"

	"golang.org/x/crypto/ssh"
)

func publicKeyFile(file string) ssh.AuthMethod {
	buffer, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	key, err := ssh.ParsePrivateKey(buffer)
	if err != nil {
		panic(err)
	}
	return ssh.PublicKeys(key)
}

func main() {
	// monitor os signal
	sigs := make(chan os.Signal)
	signal.Notify(sigs, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			publicKeyFile("/tmp/id_rsa"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", "192.168.2.5:22", sshConfig)
	if err != nil {
		panic(err)
	}
	session, err := client.NewSession()
	if err != nil {
		panic(err)
	}
	s := sshutils.NewSSHSession(session)
	go func() {
		select {
		case <-sigs:
			fmt.Println("exit")
			_ = s.Close()
		}
	}()

	// std copy
	go func() {
		select {
		case <-s.Ready():
			_, _ = io.Copy(os.Stdout, s.Stdout)
		}
	}()

	err = s.PipeExec("journalctl -fu docker")
	if err != nil {
		fmt.Println(err)
	}
}
