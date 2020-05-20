package main

import (
	"fmt"
	"io/ioutil"
	"os"

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

	// auto switch root user
	//err = sshutils.NewSSHSessionWithRoot(session, true, true, "password", "password").TerminalWithKeepAlive(10 * time.Second)

	err = sshutils.NewSSHSession(session).Terminal()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
