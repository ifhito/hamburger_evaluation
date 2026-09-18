package domain

// User is the domain representation of an account. It deliberately
// excludes the password digest: credentials never travel on the entity
// and stay inside the persistence/usecase boundary.
type User struct {
	ID       int64
	Username string
	Email    string
	Admin    bool
}
