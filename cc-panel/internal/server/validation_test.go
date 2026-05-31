package server

import "testing"

func TestCreateInputValidate(t *testing.T) {
	input := CreateInput{
		Name:     "web-01",
		Host:     "example.com",
		Port:     22,
		Username: "root",
		AuthType: AuthTypePassword,
		Password: "secret",
	}
	if err := input.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCreateInputValidateRejectsBadHost(t *testing.T) {
	input := CreateInput{
		Name:     "web-01",
		Host:     "example.com;rm -rf /",
		Port:     22,
		Username: "root",
		AuthType: AuthTypePassword,
		Password: "secret",
	}
	if err := input.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want host validation error")
	}
}
