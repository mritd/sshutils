/*
 * Copyright 2018 mritd <mritd1234@gmail.com>
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

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
	cancelChannel := make(chan os.Signal)
	signal.Notify(cancelChannel, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	sshConfig := &ssh.ClientConfig{
		User: "root",
		Auth: []ssh.AuthMethod{
			publicKeyFile("/Users/mritd/.ssh/id_rsa"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial("tcp", "192.168.1.20:22", sshConfig)
	if err != nil {
		panic(err)
	}
	session, err := client.NewSession()
	if err != nil {
		panic(err)
	}

	s := sshutils.New(session)

	// exec
	go s.PipeExec("journalctl -fu docker")

	// print error
	go func() {
		select {
		case err := <-s.ErrCh:
			fmt.Println(err)
		}
	}()

	// std copy
	go func() {
		select {
		case <-s.ReadyCh:
			io.Copy(os.Stdout, s.Stdout)
		}
	}()

	select {
	case <-s.DoneCh:
		fmt.Println("done")
	case sig := <-cancelChannel:
		switch sig {
		case syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT:
			fmt.Println("exit")
			s.Close()
		}
	}

}
