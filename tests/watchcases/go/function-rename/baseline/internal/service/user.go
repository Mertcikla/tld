package service

type User struct {
	Name string
}

func CreateUser(name string) User {
	return User{Name: name}
}
