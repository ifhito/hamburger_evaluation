class AuthController < ApplicationController
  SECRET_KEY = Rails.application.secret_key_base

  def signup
    user = User.new(signup_params)
    if user.save
      token = encode_token(user.id)
      render json: { id: user.id, username: user.username, email: user.email, token: token }, status: :created
    else
      render json: { errors: user.errors.full_messages }, status: :unprocessable_entity
    end
  end

  def login
    user = User.kept.find_by(email: params[:email])
    if user&.authenticate(params[:password])
      token = encode_token(user.id)
      render json: { token: token }, status: :ok
    else
      render json: { error: "Invalid email or password" }, status: :unauthorized
    end
  end

  def logout
    render json: { message: "Logged out successfully" }, status: :ok
  end

  private

  def signup_params
    params.permit(:username, :email, :password, :password_confirmation)
  end

  def encode_token(user_id)
    payload = { user_id: user_id, exp: 24.hours.from_now.to_i }
    JWT.encode(payload, SECRET_KEY, "HS256")
  end
end
