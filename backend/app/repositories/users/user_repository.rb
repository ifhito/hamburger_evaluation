module Users
  class UserRepository
    def kept
      User.kept
    end

    def find_kept!(id)
      User.kept.find(id)
    end

    def find_kept_by_email(email)
      User.kept.find_by(email: email)
    end

    def build(attrs)
      User.new(attrs)
    end

    def save(user)
      user.save
    end

    def update!(user, **attrs)
      user.update!(attrs)
      user
    end

    def discard!(user)
      user.discard
    end
  end
end
