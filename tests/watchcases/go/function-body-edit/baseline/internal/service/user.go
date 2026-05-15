package service

import "strings"

type User struct {
	Name string
}

func CreateUser(name string) User {
	clean := strings.TrimSpace(name)
	return User{Name: clean}
}
