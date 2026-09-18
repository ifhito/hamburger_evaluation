package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// CreateUserParams carries the fields persisted for a new user. The
// password arrives pre-hashed: repositories never see plaintext.
type CreateUserParams struct {
	Username       string
	Email          string
	PasswordDigest string
	Admin          bool
}

// UserCredentials pairs a domain user with its password digest for
// login checks. The digest intentionally never lives on domain.User.
type UserCredentials struct {
	User           domain.User
	PasswordDigest string
}

// UserRepository is the consumer-side persistence contract for auth.
// Implementations map storage errors to domain errors:
// CreateUser returns (a wrapped) domain.ErrEmailTaken on an email unique
// violation; the lookups return domain.ErrUserNotFound when no active
// (non-discarded) user matches.
type UserRepository interface {
	CreateUser(ctx context.Context, params CreateUserParams) (domain.User, error)
	GetActiveUserByEmail(ctx context.Context, email string) (UserCredentials, error)
	GetActiveUserByID(ctx context.Context, id int64) (domain.User, error)
}

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(digest, password string) error
}

// TokenIssuer issues an auth token for a user ID.
type TokenIssuer interface {
	Issue(userID int64) (string, error)
}

// TokenVerifier verifies a raw token and returns the user ID it carries.
type TokenVerifier interface {
	Verify(token string) (int64, error)
}

// Auth implements the signup/login/token-authentication use cases.
// Authentication decisions live here, not in HTTP handlers.
type Auth struct {
	users    UserRepository
	hasher   PasswordHasher
	issuer   TokenIssuer
	verifier TokenVerifier
}

func NewAuth(users UserRepository, hasher PasswordHasher, issuer TokenIssuer, verifier TokenVerifier) *Auth {
	return &Auth{users: users, hasher: hasher, issuer: issuer, verifier: verifier}
}

// maxPasswordBytes mirrors bcrypt's 72-byte input limit, which Rails'
// has_secure_password also enforces.
const maxPasswordBytes = 72

// SignupInput is the signup use case input. PasswordConfirmation is
// optional (nil when the field was absent from the request); when
// present it must equal Password, matching Rails has_secure_password.
type SignupInput struct {
	Username             string
	Email                string
	Password             string
	PasswordConfirmation *string
}

// validate returns Rails-parity full messages, empty when valid.
func (in SignupInput) validate() []string {
	var msgs []string
	if in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	if in.Email == "" {
		msgs = append(msgs, "Email can't be blank")
	}
	if in.Password == "" {
		msgs = append(msgs, "Password can't be blank")
	}
	if len(in.Password) > maxPasswordBytes {
		msgs = append(msgs, "Password is too long (maximum is 72 characters)")
	}
	if in.PasswordConfirmation != nil && *in.PasswordConfirmation != in.Password {
		msgs = append(msgs, "Password confirmation doesn't match Password")
	}
	return msgs
}

// Signup validates the input, stores the new user (always admin=false)
// and returns it together with a fresh auth token. Validation failures
// and duplicate emails surface as *domain.ValidationError.
func (a *Auth) Signup(ctx context.Context, input SignupInput) (domain.User, string, error) {
	if msgs := input.validate(); len(msgs) > 0 {
		return domain.User{}, "", &domain.ValidationError{Messages: msgs}
	}
	digest, err := a.hasher.Hash(input.Password)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("hash password: %w", err)
	}
	user, err := a.users.CreateUser(ctx, CreateUserParams{
		Username:       input.Username,
		Email:          input.Email,
		PasswordDigest: digest,
		// New users are never admins; promotion is out of signup scope.
		Admin: false,
	})
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, "", &domain.ValidationError{Messages: []string{"Email has already been taken"}}
		}
		return domain.User{}, "", fmt.Errorf("create user: %w", err)
	}
	token, err := a.issuer.Issue(user.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return user, token, nil
}

// dummyPasswordDigest is a fixed, valid bcrypt digest (of an arbitrary
// throwaway string, cost 10 like infra's hasher) used only for the dummy
// compare in Login. It matches no real password stored by this app.
const dummyPasswordDigest = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"

// Login authenticates an active user by email and password and returns
// the user with a fresh token. Unknown email and wrong password both
// yield domain.ErrInvalidCredentials.
func (a *Auth) Login(ctx context.Context, email, password string) (domain.User, string, error) {
	creds, err := a.users.GetActiveUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			// Burn one hash comparison so the unknown-email path takes
			// about as long as the wrong-password path; otherwise the
			// response-time difference would let callers enumerate
			// which emails have accounts.
			_ = a.hasher.Compare(dummyPasswordDigest, password)
			return domain.User{}, "", domain.ErrInvalidCredentials
		}
		return domain.User{}, "", fmt.Errorf("get user by email: %w", err)
	}
	if err := a.hasher.Compare(creds.PasswordDigest, password); err != nil {
		return domain.User{}, "", domain.ErrInvalidCredentials
	}
	token, err := a.issuer.Issue(creds.User.ID)
	if err != nil {
		return domain.User{}, "", fmt.Errorf("issue token: %w", err)
	}
	return creds.User, token, nil
}

// AuthenticateToken verifies rawToken and resolves its active user.
// Invalid or expired tokens and unknown or discarded users all yield
// domain.ErrUnauthenticated; infrastructure failures propagate as-is.
func (a *Auth) AuthenticateToken(ctx context.Context, rawToken string) (domain.User, error) {
	userID, err := a.verifier.Verify(rawToken)
	if err != nil {
		return domain.User{}, domain.ErrUnauthenticated
	}
	user, err := a.users.GetActiveUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return domain.User{}, domain.ErrUnauthenticated
		}
		return domain.User{}, fmt.Errorf("get user by id: %w", err)
	}
	return user, nil
}
