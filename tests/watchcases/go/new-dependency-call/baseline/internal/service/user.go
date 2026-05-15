package service

import "example.com/watchcase/dependency/internal/repository"

type Service struct {
	repo repository.Repository
}

func (s Service) CreateUser(name string) error {
	if name == "" {
		return nil
	}
	return nil
}
