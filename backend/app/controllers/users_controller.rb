class UsersController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index ]

  def index
    users = user_repository.kept
    render json: users.map { |u| UserSerializer.new(u).as_json }
  end

  def update
    user = user_repository.find_kept!(params[:id])
    authorize user
    Users::UpdateUserService.new(user: user, params: user_params).invoke
    render json: UserSerializer.new(user).as_json
  rescue ActiveRecord::RecordNotFound
    render json: { error: "User not found" }, status: :not_found
  rescue ActiveRecord::RecordInvalid => e
    render json: { errors: e.record.errors.full_messages }, status: :unprocessable_entity
  end

  def destroy
    user = user_repository.find_kept!(params[:id])
    authorize user
    Users::DeleteUserService.new(user: user).invoke
    head :no_content
  rescue ActiveRecord::RecordNotFound
    render json: { error: "User not found" }, status: :not_found
  end

  private

  def user_params
    params.require(:user).permit(:username, :email, :password, :password_confirmation)
  end

  def user_repository
    Users::UserRepository.new
  end
end
