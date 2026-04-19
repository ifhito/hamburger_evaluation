module Users
  class UpdateUserService
    def initialize(user:, params:)
      @user   = user
      @params = params
    end

    def invoke
      @user.update!(@params)
      @user
    end
  end
end
