module Users
  class DeleteUserService
    def initialize(user:)
      @user = user
    end

    def invoke
      @user.discard
    end
  end
end
