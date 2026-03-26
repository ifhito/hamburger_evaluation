class UsersController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index ]

  def index
    users = User.kept
    render json: users.map { |u| UserSerializer.new(u).as_json }
  end

  def update
    user = User.kept.find(params[:id])
    authorize user
    if user.update(user_params)
      render json: UserSerializer.new(user).as_json
    else
      render json: { errors: user.errors.full_messages }, status: :unprocessable_entity
    end
  rescue ActiveRecord::RecordNotFound
    render json: { error: "User not found" }, status: :not_found
  end

  def destroy
    user = User.kept.find(params[:id])
    authorize user
    user.discard
    head :no_content
  rescue ActiveRecord::RecordNotFound
    render json: { error: "User not found" }, status: :not_found
  end

  private

  def user_params
    params.require(:user).permit(:username, :email, :password, :password_confirmation)
  end
end
