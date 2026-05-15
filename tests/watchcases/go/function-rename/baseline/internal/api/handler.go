package api

import "example.com/watchcase/rename/internal/service"

func HandleCreate(name string) service.User {
	return service.CreateUser(name)
}
