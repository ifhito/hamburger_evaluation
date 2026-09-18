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

// Manages reports whether the user may manage (edit or delete) the
// account with the given id: self-management only, an admin gets no pass
// — mirrors Rails UserEntity#manages? behind UserPolicy update?/destroy?.
func (u User) Manages(id int64) bool { return u.ID == id }
