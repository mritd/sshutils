package main

import (
	"io/ioutil"

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
			publicKeyFile("/Users/mritd/.ssh/id_rsa"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", "10.211.55.11:22", sshConfig)
	if err != nil {
		panic(err)
	}

	scp, err := sshutils.NewSCPClient(client)
	if err != nil {
		panic(err)
	}
	err = scp.CopyLocal2Remote("~/tmp/docker.service", "~/.ssh/id_rsa.pub", "~")
	if err != nil {
		panic(err)
	}

	err = scp.CopyLocal2Remote("~/tmp/CEFI/EFI/BOOT/BOOTX64.efi", "~/tmp/EFI", "~")
	if err != nil {
		panic(err)
	}

	err = scp.CopyRemote2Local("~/EFI", "~/tmp/mcptest")
	if err != nil {
		panic(err)
	}

	err = scp.CopyRemote2Local("~/EFI", "~/tmp/mcptest")
	if err != nil {
		panic(err)
	}

	err = scp.CopyRemote2Local("~/BOOTX64.efi", "~/tmp")
	if err != nil {
		panic(err)
	}

	err = scp.CopyRemote2Local("~/BOOTX64.efi", "~/tmp/aaaa")
	if err != nil {
		panic(err)
	}
}
