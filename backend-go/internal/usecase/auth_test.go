package usecase_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
	"github.com/ifhito/hamburger_evaluation/backend-go/internal/usecase"
)

// fakeUserRepo is a hand-written usecase.UserRepository test double.
// Unset behaviors panic so tests fail loudly on unexpected calls.
type fakeUserRepo struct {
	createUser func(ctx context.Context, params usecase.CreateUserParams) (domain.User, error)
	getByEmail func(ctx context.Context, email string) (usecase.UserCredentials, error)
	getByID    func(ctx context.Context, id int64) (domain.User, error)
}

func (f *fakeUserRepo) CreateUser(ctx context.Context, params usecase.CreateUserParams) (domain.User, error) {
	if f.createUser == nil {
		panic("unexpected CreateUser call")
	}
	return f.createUser(ctx, params)
}

func (f *fakeUserRepo) GetActiveUserByEmail(ctx context.Context, email string) (usecase.UserCredentials, error) {
	if f.getByEmail == nil {
		panic("unexpected GetActiveUserByEmail call")
	}
	return f.getByEmail(ctx, email)
}

func (f *fakeUserRepo) GetActiveUserByID(ctx context.Context, id int64) (domain.User, error) {
	if f.getByID == nil {
		panic("unexpected GetActiveUserByID call")
	}
	return f.getByID(ctx, id)
}

// fakeHasher marks digests deterministically so tests can assert what
// was stored and compared without real bcrypt work.
type fakeHasher struct{}

func (fakeHasher) Hash(password string) (string, error) { return "digest(" + password + ")", nil }

func (fakeHasher) Compare(digest, password string) error {
	if digest != "digest("+password+")" {
		return errors.New("password mismatch")
	}
	return nil
}

type fakeIssuer struct{}

func (fakeIssuer) Issue(userID int64) (string, error) {
	return fmt.Sprintf("token-for-%d", userID), nil
}

type fakeVerifier struct {
	verify func(token string) (int64, error)
}

func (f fakeVerifier) Verify(token string) (int64, error) { return f.verify(token) }

func strPtr(s string) *string { return &s }

// assertValidationError fails the test unless err is a *domain.ValidationError
// carrying exactly wantMsgs.
func assertValidationError(t *testing.T, err error, wantMsgs []string) {
	t.Helper()
	var vErr *domain.ValidationError
	if !errors.As(err, &vErr) {
		t.Fatalf("error = %v (%T), want *domain.ValidationError", err, err)
	}
	if !reflect.DeepEqual(vErr.Messages, wantMsgs) {
		t.Fatalf("validation messages = %q, want %q", vErr.Messages, wantMsgs)
	}
}

func TestAuthSignupValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    usecase.SignupInput
		wantMsgs []string
	}{
		{
			name:     "blank username",
			input:    usecase.SignupInput{Email: "a@example.com", Password: "password123"},
			wantMsgs: []string{"Username can't be blank"},
		},
		{
			name:     "blank email",
			input:    usecase.SignupInput{Username: "alice", Password: "password123"},
			wantMsgs: []string{"Email can't be blank"},
		},
		{
			name:     "blank password",
			input:    usecase.SignupInput{Username: "alice", Email: "a@example.com"},
			wantMsgs: []string{"Password can't be blank"},
		},
		{
			name: "password longer than 72 bytes",
			input: usecase.SignupInput{
				Username: "alice",
				Email:    "a@example.com",
				Password: strings.Repeat("a", 73),
			},
			wantMsgs: []string{"Password is too long (maximum is 72 characters)"},
		},
		{
			name: "password confirmation mismatch",
			input: usecase.SignupInput{
				Username:             "alice",
				Email:                "a@example.com",
				Password:             "password123",
				PasswordConfirmation: strPtr("password124"),
			},
			wantMsgs: []string{"Password confirmation doesn't match Password"},
		},
		{
			name:  "all fields blank",
			input: usecase.SignupInput{},
			wantMsgs: []string{
				"Username can't be blank",
				"Email can't be blank",
				"Password can't be blank",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The repository must not be reached on validation failure.
			auth := usecase.NewAuth(&fakeUserRepo{}, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
			_, _, err := auth.Signup(context.Background(), tt.input)
			assertValidationError(t, err, tt.wantMsgs)
		})
	}
}

func TestAuthSignup(t *testing.T) {
	t.Run("valid input creates non-admin user with hashed password and returns token", func(t *testing.T) {
		var gotParams usecase.CreateUserParams
		repo := &fakeUserRepo{
			createUser: func(_ context.Context, params usecase.CreateUserParams) (domain.User, error) {
				gotParams = params
				return domain.User{ID: 1, Username: params.Username, Email: params.Email, Admin: params.Admin}, nil
			},
		}
		auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})

		user, token, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username:             "alice",
			Email:                "a@example.com",
			Password:             "password123",
			PasswordConfirmation: strPtr("password123"),
		})
		if err != nil {
			t.Fatalf("Signup returned error: %v", err)
		}
		wantParams := usecase.CreateUserParams{
			Username:       "alice",
			Email:          "a@example.com",
			PasswordDigest: "digest(password123)",
			Admin:          false,
		}
		if gotParams != wantParams {
			t.Fatalf("CreateUser params = %+v, want %+v", gotParams, wantParams)
		}
		wantUser := domain.User{ID: 1, Username: "alice", Email: "a@example.com", Admin: false}
		if user != wantUser {
			t.Fatalf("Signup user = %+v, want %+v", user, wantUser)
		}
		if token != "token-for-1" {
			t.Fatalf("Signup token = %q, want %q", token, "token-for-1")
		}
	})

	t.Run("nil password confirmation is accepted", func(t *testing.T) {
		repo := &fakeUserRepo{
			createUser: func(_ context.Context, params usecase.CreateUserParams) (domain.User, error) {
				return domain.User{ID: 2, Username: params.Username, Email: params.Email}, nil
			},
		}
		auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "password123",
		})
		if err != nil {
			t.Fatalf("Signup returned error: %v", err)
		}
	})

	t.Run("duplicate email surfaces as validation error", func(t *testing.T) {
		repo := &fakeUserRepo{
			createUser: func(context.Context, usecase.CreateUserParams) (domain.User, error) {
				return domain.User{}, fmt.Errorf("create user: %w", domain.ErrEmailTaken)
			},
		}
		auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "password123",
		})
		assertValidationError(t, err, []string{"Email has already been taken"})
	})

	t.Run("other repository errors propagate", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		repo := &fakeUserRepo{
			createUser: func(context.Context, usecase.CreateUserParams) (domain.User, error) {
				return domain.User{}, repoErr
			},
		}
		auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Signup(context.Background(), usecase.SignupInput{
			Username: "alice",
			Email:    "a@example.com",
			Password: "password123",
		})
		if !errors.Is(err, repoErr) {
			t.Fatalf("error = %v, want wrapped %v", err, repoErr)
		}
	})
}

func TestAuthLogin(t *testing.T) {
	activeUser := domain.User{ID: 7, Username: "alice", Email: "a@example.com"}
	repo := &fakeUserRepo{
		getByEmail: func(_ context.Context, email string) (usecase.UserCredentials, error) {
			if email != "a@example.com" {
				return usecase.UserCredentials{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			}
			return usecase.UserCredentials{User: activeUser, PasswordDigest: "digest(password123)"}, nil
		},
	}
	auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, fakeVerifier{})

	tests := []struct {
		name      string
		email     string
		password  string
		wantErr   error
		wantToken string
	}{
		{name: "correct credentials", email: "a@example.com", password: "password123", wantToken: "token-for-7"},
		{name: "wrong password", email: "a@example.com", password: "nope", wantErr: domain.ErrInvalidCredentials},
		{name: "unknown email", email: "b@example.com", password: "password123", wantErr: domain.ErrInvalidCredentials},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, token, err := auth.Login(context.Background(), tt.email, tt.password)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Login error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Login returned error: %v", err)
			}
			if user != activeUser {
				t.Fatalf("Login user = %+v, want %+v", user, activeUser)
			}
			if token != tt.wantToken {
				t.Fatalf("Login token = %q, want %q", token, tt.wantToken)
			}
		})
	}

	t.Run("repository failure propagates, not invalid credentials", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		failing := &fakeUserRepo{
			getByEmail: func(context.Context, string) (usecase.UserCredentials, error) {
				return usecase.UserCredentials{}, repoErr
			},
		}
		auth := usecase.NewAuth(failing, fakeHasher{}, fakeIssuer{}, fakeVerifier{})
		_, _, err := auth.Login(context.Background(), "a@example.com", "password123")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want wrapped %v", err, repoErr)
		}
	})
}

func TestAuthAuthenticateToken(t *testing.T) {
	activeUser := domain.User{ID: 7, Username: "alice", Email: "a@example.com"}
	repo := &fakeUserRepo{
		getByID: func(_ context.Context, id int64) (domain.User, error) {
			if id != activeUser.ID {
				// Unknown and discarded users are both "not found"
				// at the repository boundary.
				return domain.User{}, fmt.Errorf("lookup: %w", domain.ErrUserNotFound)
			}
			return activeUser, nil
		},
	}
	verifier := fakeVerifier{verify: func(token string) (int64, error) {
		switch token {
		case "valid-active":
			return activeUser.ID, nil
		case "valid-discarded":
			return 8, nil
		default:
			// Stands in for tampered, expired, and wrong-algorithm
			// tokens, all rejected by the real verifier (see infra
			// JWT tests).
			return 0, errors.New("invalid token")
		}
	}}
	auth := usecase.NewAuth(repo, fakeHasher{}, fakeIssuer{}, verifier)

	tests := []struct {
		name    string
		token   string
		wantErr error
	}{
		{name: "valid token of active user", token: "valid-active"},
		{name: "invalid token", token: "tampered-or-expired", wantErr: domain.ErrUnauthenticated},
		{name: "token of unknown or discarded user", token: "valid-discarded", wantErr: domain.ErrUnauthenticated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := auth.AuthenticateToken(context.Background(), tt.token)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("AuthenticateToken error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("AuthenticateToken returned error: %v", err)
			}
			if user != activeUser {
				t.Fatalf("AuthenticateToken user = %+v, want %+v", user, activeUser)
			}
		})
	}

	t.Run("repository failure propagates, not unauthenticated", func(t *testing.T) {
		repoErr := errors.New("connection lost")
		failing := &fakeUserRepo{
			getByID: func(context.Context, int64) (domain.User, error) {
				return domain.User{}, repoErr
			},
		}
		auth := usecase.NewAuth(failing, fakeHasher{}, fakeIssuer{}, verifier)
		_, err := auth.AuthenticateToken(context.Background(), "valid-active")
		if !errors.Is(err, repoErr) || errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("AuthenticateToken error = %v, want wrapped %v", err, repoErr)
		}
	})
}
