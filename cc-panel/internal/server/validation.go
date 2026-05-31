package server

import (
	"fmt"
	"net"
	"strings"
)

func (input CreateInput) Validate() error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if input.Port < 0 || input.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if strings.TrimSpace(input.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if err := validateHost(input.Host); err != nil {
		return err
	}
	return validateAuth(input.AuthType, input.Password, input.PrivateKey)
}

func (input UpdateInput) Validate() error {
	if strings.TrimSpace(input.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(input.Host) == "" {
		return fmt.Errorf("host is required")
	}
	if input.Port <= 0 || input.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if strings.TrimSpace(input.Username) == "" {
		return fmt.Errorf("username is required")
	}
	if err := validateHost(input.Host); err != nil {
		return err
	}
	if input.Password != nil || input.PrivateKey != nil {
		password := ""
		privateKey := ""
		if input.Password != nil {
			password = *input.Password
		}
		if input.PrivateKey != nil {
			privateKey = *input.PrivateKey
		}
		return validateAuth(input.AuthType, password, privateKey)
	}
	if input.AuthType != AuthTypePassword && input.AuthType != AuthTypePrivateKey {
		return fmt.Errorf("auth_type must be password or private_key")
	}
	return nil
}

func validateAuth(authType, password, privateKey string) error {
	switch authType {
	case AuthTypePassword:
		if password == "" {
			return fmt.Errorf("password is required for password auth")
		}
	case AuthTypePrivateKey:
		if privateKey == "" {
			return fmt.Errorf("private_key is required for private_key auth")
		}
	default:
		return fmt.Errorf("auth_type must be password or private_key")
	}
	return nil
}

func validateHost(host string) error {
	if net.ParseIP(host) != nil {
		return nil
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" {
			return fmt.Errorf("host is invalid")
		}
		for _, r := range part {
			if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
				return fmt.Errorf("host is invalid")
			}
		}
	}
	return nil
}
