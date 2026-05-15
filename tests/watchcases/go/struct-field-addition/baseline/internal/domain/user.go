package domain

type User struct {
	ID   string
	Name string
}

func NewUser(id, name string) User {
	return User{ID: id, Name: name}
}
