class UsersController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index ]

  def index
    users = User.kept
    render json: users.map { |u| UserSerializer.new(u).as_json }
  end

  def update
    if current_user.update(user_params)
      render json: UserSerializer.new(current_user).as_json
    else
      render json: { errors: current_user.errors.full_messages }, status: :unprocessable_entity
    end
  end

  def destroy
    current_user.discard
    head :no_content
  end

  private

  def user_params
    params.require(:user).permit(:username, :email, :password, :password_confirmation)
  end
end
