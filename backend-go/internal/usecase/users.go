package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/ifhito/hamburger_evaluation/backend-go/internal/domain"
)

// ProfileChanges carries the optional column-scoped profile updates of
// UsersRepository.UpdateUserProfile: a nil field is left untouched. The
// password arrives pre-hashed, like CreateUserParams — repositories never
// see plaintext.
type ProfileChanges struct {
	Username       *string
	Email          *string
	PasswordDigest *string
}

// UsersRepository is the consumer-side persistence contract for the user
// management use cases. Implementations map storage errors to domain
// errors: the lookup and the writes return (a wrapped)
// domain.ErrUserNotFound when no active (non-discarded) user matches, and
// UpdateUserProfile returns (a wrapped) domain.ErrEmailTaken on an email
// unique violation.
type UsersRepository interface {
	// ListActiveUsers returns every kept user, id ascending (no
	// pagination — Rails parity: the index returns all kept users).
	ListActiveUsers(ctx context.Context) ([]domain.User, error)
	// GetActiveUserByID returns the non-discarded user with the given id.
	GetActiveUserByID(ctx context.Context, id int64) (domain.User, error)
	// UpdateUserProfile applies the present fields of changes to the
	// still kept user under id atomically and returns the stored user;
	// zero present fields are a plain lookup (200 no-op, Rails parity).
	UpdateUserProfile(ctx context.Context, id int64, changes ProfileChanges) (domain.User, error)
	// DiscardUser soft-deletes the user (never a hard DELETE) and keeps
	// the derived burger stats consistent.
	DiscardUser(ctx context.Context, id int64) error
}

// Users implements the user management use cases: the public index and
// the self-only profile update and account deletion.
type Users struct {
	repo   UsersRepository
	hasher PasswordHasher
}

func NewUsers(repo UsersRepository, hasher PasswordHasher) *Users {
	return &Users{repo: repo, hasher: hasher}
}

// List returns every kept user, id ascending.
func (s *Users) List(ctx context.Context) ([]domain.User, error) {
	users, err := s.repo.ListActiveUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	return users, nil
}

// UpdateUserInput is the profile update input. Every field is optional
// (nil when absent from the request); absent fields are left untouched —
// a partial update, matching Rails strong params.
type UpdateUserInput struct {
	Username             *string
	Email                *string
	Password             *string
	PasswordConfirmation *string
}

// passwordPresent reports whether the input asks for a password change.
// Rails has_secure_password nuance: a password provided as the EMPTY
// STRING is treated as absent (`password=("")` leaves the digest
// untouched and raises no error), so nil and "" both mean "no change".
func (in UpdateUserInput) passwordPresent() bool {
	return in.Password != nil && *in.Password != ""
}

// validate returns Rails-parity full messages, empty when valid.
func (in UpdateUserInput) validate() []string {
	var msgs []string
	if in.Username != nil && *in.Username == "" {
		msgs = append(msgs, "Username can't be blank")
	}
	if in.Email != nil && *in.Email == "" {
		msgs = append(msgs, "Email can't be blank")
	}
	if in.passwordPresent() && len(*in.Password) > maxPasswordBytes {
		msgs = append(msgs, "Password is too long (maximum is 72 characters)")
	}
	if in.PasswordConfirmation != nil {
		password := ""
		if in.Password != nil {
			password = *in.Password
		}
		if *in.PasswordConfirmation != password {
			msgs = append(msgs, "Password confirmation doesn't match Password")
		}
	}
	return msgs
}

// Update edits the target user's profile in the issue #16 AC2 check
// order (deliberately diverging from this branch's Rails controller,
// which ignores the path id and operates on current_user): load
// (404 for missing and discarded alike — even for a non-owner), the
// domain self-management rule (403), input validation (422), then the
// column-scoped write. A taken email surfaces as *domain.ValidationError,
// like signup.
func (s *Users) Update(ctx context.Context, viewer domain.User, targetID int64, input UpdateUserInput) (domain.User, error) {
	target, err := s.repo.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.User{}, domain.ErrForbidden
	}
	if msgs := input.validate(); len(msgs) > 0 {
		return domain.User{}, &domain.ValidationError{Messages: msgs}
	}
	changes := ProfileChanges{Username: input.Username, Email: input.Email}
	if input.passwordPresent() {
		digest, err := s.hasher.Hash(*input.Password)
		if err != nil {
			return domain.User{}, fmt.Errorf("hash password: %w", err)
		}
		changes.PasswordDigest = &digest
	}
	updated, err := s.repo.UpdateUserProfile(ctx, targetID, changes)
	if err != nil {
		if errors.Is(err, domain.ErrEmailTaken) {
			return domain.User{}, &domain.ValidationError{Messages: []string{"Email has already been taken"}}
		}
		return domain.User{}, fmt.Errorf("update user: %w", err)
	}
	return updated, nil
}

// Delete soft-deletes the target user's account: load (404, even for a
// non-owner), the domain self-management rule (403), then the discard —
// never a hard DELETE.
func (s *Users) Delete(ctx context.Context, viewer domain.User, targetID int64) error {
	target, err := s.repo.GetActiveUserByID(ctx, targetID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if !viewer.Manages(target.ID) {
		return domain.ErrForbidden
	}
	if err := s.repo.DiscardUser(ctx, targetID); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}
