package sshx

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type Credentials struct {
	Host       string
	Port       int
	Username   string
	AuthType   string
	Password   string
	PrivateKey string
	Timeout    time.Duration
}

type Result struct {
	Stdout string
	Stderr string
}

type Executor interface {
	Run(ctx context.Context, creds Credentials, command string) (Result, error)
	RunScript(ctx context.Context, creds Credentials, script string) (Result, error)
}

type SSHExecutor struct{}

func NewSSHExecutor() *SSHExecutor {
	return &SSHExecutor{}
}

func (e *SSHExecutor) Run(ctx context.Context, creds Credentials, command string) (Result, error) {
	return e.run(ctx, creds, command, "")
}

func (e *SSHExecutor) RunScript(ctx context.Context, creds Credentials, script string) (Result, error) {
	return e.run(ctx, creds, "", script)
}

func (e *SSHExecutor) run(ctx context.Context, creds Credentials, command, script string) (Result, error) {
	auth, err := authMethod(creds)
	if err != nil {
		return Result{}, err
	}
	timeout := creds.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	config := &ssh.ClientConfig{
		User:            creds.Username,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}
	address := net.JoinHostPort(creds.Host, fmt.Sprintf("%d", creds.Port))

	type runResult struct {
		result Result
		err    error
	}
	done := make(chan runResult, 1)
	go func() {
		client, err := ssh.Dial("tcp", address, config)
		if err != nil {
			done <- runResult{err: fmt.Errorf("dial ssh: %w", err)}
			return
		}
		defer client.Close()

		session, err := client.NewSession()
		if err != nil {
			done <- runResult{err: fmt.Errorf("create ssh session: %w", err)}
			return
		}
		defer session.Close()

		var stdout bytes.Buffer
		var stderr bytes.Buffer
		session.Stdout = &stdout
		session.Stderr = &stderr
		if script != "" {
			session.Stdin = strings.NewReader(script)
			err = session.Run("/bin/sh -s")
		} else {
			err = session.Run(command)
		}
		done <- runResult{result: Result{Stdout: stdout.String(), Stderr: stderr.String()}, err: err}
	}()

	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case result := <-done:
		return result.result, result.err
	}
}

func authMethod(creds Credentials) (ssh.AuthMethod, error) {
	switch creds.AuthType {
	case "password":
		if creds.Password == "" {
			return nil, fmt.Errorf("password auth requires password")
		}
		return ssh.Password(creds.Password), nil
	case "private_key":
		if creds.PrivateKey == "" {
			return nil, fmt.Errorf("private_key auth requires private key")
		}
		signer, err := ssh.ParsePrivateKey([]byte(creds.PrivateKey))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		return ssh.PublicKeys(signer), nil
	default:
		return nil, fmt.Errorf("unsupported auth_type %q", creds.AuthType)
	}
}
