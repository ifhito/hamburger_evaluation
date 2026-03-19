module Authenticatable
  extend ActiveSupport::Concern

  SECRET_KEY = Rails.application.secret_key_base

  included do
    before_action :authenticate_user!
  end

  def authenticate_user!
    token = request.headers["Authorization"]&.split(" ")&.last
    payload = JWT.decode(token, SECRET_KEY, true, algorithm: "HS256").first
    @current_user = User.kept.find(payload["user_id"])
  rescue JWT::DecodeError, ActiveRecord::RecordNotFound
    render json: { error: "Unauthorized" }, status: :unauthorized
  end

  def current_user
    @current_user
  end
end
