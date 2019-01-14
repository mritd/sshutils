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

type SSHSession struct {
	// ssh session
	session *ssh.Session
	// error channel return the error when command exec failed
	errCh chan error
	// for PipeExec, this channel will be read when stdin、stdout ready
	readyCh chan int
	// for PipeExec, this channel will be read when command exec finished
	doneCh chan int
	// for Interactive shell, this channel will be read when shell ready
	shellDoneCh chan int
	// shell command exit message
	exitMsg string
	// if true, we will auto switch to root user
	suRoot bool
	// if true, use the 'sudo' command to switch root user
	useSudo bool
	// not send user password when run `sudo su - root`
	noPasswordSudo bool
	// for auto switch root user(use sudo)
	userPassword string
	// for auto switch root user
	rootPassword string
	// delay the specified time execution command when automatically
	// switching the root user to ensure that terminal stdout outputs correctly
	cmdDelay time.Duration
	Stdout   io.Reader
	Stdin    io.Writer
	Stderr   io.Reader
}

func (s *SSHSession) Error() <-chan error {
	return s.errCh
}

func (s *SSHSession) Ready() <-chan int {
	return s.readyCh
}

func (s *SSHSession) Done() <-chan int {
	return s.doneCh
}

// close the session
func (s *SSHSession) Close() error {

	var err error
	pw, ok := s.session.Stdout.(*io.PipeWriter)
	if ok {
		err = pw.Close()
		if err != nil {
			return err
		}
	}

	pr, ok := s.session.Stdin.(*io.PipeReader)
	if ok {
		err = pr.Close()
		if err != nil {
			return err
		}
	}

	err = s.session.Close()
	if err != nil {
		return err
	}
	return nil
}

// update shell terminal size in background
func (s *SSHSession) updateTerminalSize() {

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

				err = s.session.WindowChange(currTermHeight, currTermWidth)
				if err != nil {
					fmt.Printf("Unable to send window-change reqest: %s.", err)
					continue
				}

				termWidth, termHeight = currTermWidth, currTermHeight

			}
		}
	}()

}

func (s *SSHSession) ShellDone() <-chan int {
	return s.shellDoneCh
}

// open a interactive shell
func (s *SSHSession) Terminal() error {
	return s.TerminalWithKeepAlive(0)
}

// open a interactive shell with keepalive
func (s *SSHSession) TerminalWithKeepAlive(serverAliveInterval time.Duration) error {

	defer func() {
		if s.exitMsg == "" {
			_, _ = fmt.Fprintln(os.Stdout, "the connection was closed on the remote side on ", time.Now().Format(time.RFC822))
		} else {
			_, _ = fmt.Fprintln(os.Stdout, s.exitMsg)
		}
	}()

	fd := int(os.Stdin.Fd())
	state, err := terminal.MakeRaw(fd)
	if err != nil {
		return err
	}
	defer func() {
		_ = terminal.Restore(fd, state)
	}()

	// get terminal size
	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		return err
	}

	// default to xterm-256color
	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	// request pty
	err = s.session.RequestPty(termType, termHeight, termWidth, ssh.TerminalModes{})
	if err != nil {
		return err
	}

	// update shell terminal size in background
	s.updateTerminalSize()

	// get pipe stdin
	s.Stdin, err = s.session.StdinPipe()
	if err != nil {
		return err
	}

	// get pipe stdout
	s.Stdout, err = s.session.StdoutPipe()
	if err != nil {
		return err
	}

	// get pipe stderr
	s.Stderr, err = s.session.StderrPipe()

	// async copy
	go func() {
		_, _ = io.Copy(os.Stderr, s.Stderr)
	}()
	go func() {
		_, _ = io.Copy(os.Stdout, s.Stdout)
	}()
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

	// keepalive
	if serverAliveInterval > 0 {
		go func() {
			for {
				select {
				case <-time.Tick(serverAliveInterval):
					_, err := s.session.SendRequest("keepalive@mritd.me", true, nil)
					if err != nil {
						fmt.Println(err)
					}
				}
			}

		}()
	}

	// open shell
	err = s.session.Shell()
	if err != nil {
		return err
	}

	s.shellDoneCh <- 1

	// auto switch root user
	if s.suRoot {
		go func() {
			// delayed execution ensures that welcome messages have been printed to the terminal
			time.Sleep(s.cmdDelay)

			if s.useSudo {
				_, err := s.Stdin.Write([]byte("sudo su - root && exit\n"))
				if err != nil {
					panic(err)
				}
				if !s.noPasswordSudo {
					// waiting the 'Password:' message have been printed to the terminal
					time.Sleep(s.cmdDelay)
					_, err = s.Stdin.Write([]byte(s.userPassword + "\n"))
					if err != nil {
						panic(err)
					}
				}
			} else {
				_, err := s.Stdin.Write([]byte("su - root && exit\n"))
				if err != nil {
					panic(err)
				}
				// waiting the 'Password:' message have been printed to the terminal
				time.Sleep(s.cmdDelay)
				_, err = s.Stdin.Write([]byte(s.rootPassword + "\n"))
				if err != nil {
					panic(err)
				}
			}

			// waiting switch root user done
			time.Sleep(s.cmdDelay)
			// clean stdout cmd info
			if s.noPasswordSudo {
				_, err = s.Stdin.Write([]byte(`echo -e "\033[1A\033[2K\033[1A\033[2K\033[1A\033[2K"` + "\n"))
			} else {
				_, err = s.Stdin.Write([]byte(`echo -e "\033[1A\033[2K\033[1A\033[2K\033[1A\033[2K\033[1A\033[2K"` + "\n"))
			}
			if err != nil {
				panic(err)
			}
		}()
	}

	err = s.session.Wait()
	if err != nil {
		return err
	}
	return nil
}

// pipe exec
func (s *SSHSession) PipeExec(cmd string) {

	defer func() {
		s.doneCh <- 1
		close(s.errCh)
	}()

	fd := int(os.Stdin.Fd())
	termWidth, termHeight, err := terminal.GetSize(fd)
	if err != nil {
		s.errCh <- err
		return
	}

	// default to xterm-256color
	termType := os.Getenv("TERM")
	if termType == "" {
		termType = "xterm-256color"
	}

	// request pty
	err = s.session.RequestPty(termType, termHeight, termWidth, ssh.TerminalModes{})
	if err != nil {
		s.errCh <- err
		return
	}

	// write to pw
	pr, pw := io.Pipe()
	s.session.Stdout = pw
	s.session.Stderr = pw
	s.Stdout = pr
	s.Stderr = pr

	s.readyCh <- 1

	defer func() {
		_ = pw.Close()
	}()
	err = s.session.Run(cmd)
	if err != nil {
		s.errCh <- err
	}

}

// New Session
func NewSSHSession(session *ssh.Session) *SSHSession {
	return &SSHSession{
		session:     session,
		errCh:       make(chan error, 1),
		readyCh:     make(chan int, 1),
		doneCh:      make(chan int, 1),
		shellDoneCh: make(chan int, 1),
	}
}

// New Session and auto switch root user
func NewSSHSessionWithRoot(session *ssh.Session, useSudo, noPasswordSudo bool, rootPassword, userPassword string) *SSHSession {
	return NewSSHSessionWithRootAndCmdDelay(session, useSudo, noPasswordSudo, rootPassword, userPassword, time.Second/10)
}

// New Session and auto switch root user(support custom switch cmd delay)
func NewSSHSessionWithRootAndCmdDelay(session *ssh.Session, useSudo, noPasswordSudo bool, rootPassword, userPassword string, cmdDelay time.Duration) *SSHSession {

	// default to 0.1s
	if cmdDelay < time.Second/10 {
		cmdDelay = time.Second / 10
	}

	return &SSHSession{
		session:        session,
		errCh:          make(chan error, 1),
		readyCh:        make(chan int, 1),
		doneCh:         make(chan int, 1),
		shellDoneCh:    make(chan int, 1),
		suRoot:         true,
		useSudo:        useSudo,
		noPasswordSudo: noPasswordSudo,
		userPassword:   userPassword,
		rootPassword:   rootPassword,
		cmdDelay:       cmdDelay,
	}
}
