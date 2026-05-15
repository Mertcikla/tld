package service

type User struct {
	Name string
}

func CreateUser(name string) User {
	return User{Name: normalizeName(name)}
}

func normalizeName(name string) string {
	if name == "" {
		return "anonymous"
	}
	return name
}
