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

package sshutils

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"golang.org/x/crypto/ssh"
)

type sshSession struct {
	Session *ssh.Session
	ErrCh   chan error
	ReadyCh chan int
	DoneCh  chan int
	exitMsg string
	Stdout  io.Reader
	Stdin   io.Writer
	Stderr  io.Reader
}

func (s *sshSession) Close() error {

	var err error
	pw, ok := s.Session.Stdout.(*io.PipeWriter)
	if ok {
		err = pw.Close()
		if err != nil {
			return err
		}
	}

	pr, ok := s.Session.Stdin.(*io.PipeReader)
	if ok {
		err = pr.Close()
		if err != nil {
			return err
		}
	}

	err = s.Session.Close()
	if err != nil {
		return err
	}
	return nil
}

func (s *sshSession) updateTerminalSize() {

	go func() {
		// SIGWINCH is sent to the process when the window size of the terminal has
		// changed.
		sigwinchCh := make(chan os.Signal, 1)
		signal.Notify(sigwinchCh, syscall.SIGWINCH)

		fd := int(os.Stdin.Fd())
		termWidth, termHeight, err := terminal.GetSize(fd)
		if err != nil {
			fmt.Println(err)
		}

		for {
			select {
			// The client updated the size of the local PTY. This change needs to occur
			// on the server side PTY as well.
			case sigwinch := <-sigwinchCh:
				if sigwinch == nil {
					return
				}
				currTermWidth, currTermHeight, err := terminal.GetSize(fd)

				// Terminal size has not changed, don's do anything.
				if currTermHeight == termHeight && currTermWidth == termWidth {
					continue
				}

				s.Session.WindowChange(currTermHeight, currTermWidth)
				if err != nil {
					fmt.Printf("Unable to send window-change reqest: %s.", err)
					continue
				}

				termWidth, termHeight = currTermWidth, currTermHeight

			}
		}
	}()

}

func (s *sshSession) Terminal() error {

	defer func() {
		if s.exitMsg == "" {
			fmt.Fprintln(os.Stdout, "the connection was closed on the remote side on ", time.Now().Format(time.RFC822))
		} else {
			fmt.Fprintln(os.Stdout, s.exitMsg)
		}
	}()

	fd := int(os.Stdin.Fd())
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer terminal.Restore(fd, state)

	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		return err
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	err = s.Session.RequestPty(termType, termHeight, termWidth, ssh.TerminalModes{})
	if err != nil {
		return err
	}

	s.updateTerminalSize()

	s.Stdin, err = s.Session.StdinPipe()
	if err != nil {
		return err
	}
	s.Stdout, err = s.Session.StdoutPipe()
	if err != nil {
		return err
	}
	s.Stderr, err = s.Session.StderrPipe()

	go io.Copy(os.Stderr, s.Stderr)
	go io.Copy(os.Stdout, s.Stdout)
	go func() {
		buf := make([]byte, 128)
		for {
			n, err := os.Stdin.Read(buf)
			if err != nil {
				fmt.Println(err)
				return
			}
			if n > 0 {
				_, err = s.Stdin.Write(buf[:n])
				if err != nil {
					fmt.Println(err)
					s.exitMsg = err.Error()
					return
				}
			}
		}
	}()

	err = s.Session.Shell()
	if err != nil {
		return err
	}
	err = s.Session.Wait()
	if err != nil {
		return err
	}
	return nil
}

func (s *sshSession) PipeExec(cmd string) {

	defer func() {
		s.DoneCh <- 1
		close(s.ErrCh)
	}()

	fd := int(os.Stdin.Fd())
	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		s.ErrCh <- err
		return
	}

	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	err = s.Session.RequestPty(termType, termHeight, termWidth, ssh.TerminalModes{})
	if err != nil {
		s.ErrCh <- err
		return
	}

	// write to pw
	pr, pw := io.Pipe()
	s.Session.Stdout = pw
	s.Session.Stderr = pw
	s.Stdout = pr
	s.Stderr = pr

	s.ReadyCh <- 1

	defer func() {
		pw.Close()
	}()
	err = s.Session.Run(cmd)
	if err != nil {
		s.ErrCh <- err
	}

}

func New(session *ssh.Session) *sshSession {
	return &sshSession{
		Session: session,
		ErrCh:   make(chan error),
		ReadyCh: make(chan int),
		DoneCh:  make(chan int),
	}
}
