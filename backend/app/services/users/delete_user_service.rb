module Users
  class DeleteUserService
    def initialize(user:, repository: Users::UserRepository.new)
      @user = user
      @repository = repository
    end

    def invoke
      @repository.discard!(@user)
    end
  end
end
