module Users
  class UpdateUserService
    def initialize(user:, params:, repository: Users::UserRepository.new)
      @user = user
      @params = params
      @repository = repository
    end

    def invoke
      @repository.update!(@user, **@params.to_h.symbolize_keys)
      @user
    end
  end
end
