module Authenticatable
  extend ActiveSupport::Concern

  SECRET_KEY = Rails.application.secret_key_base

  included do
    before_action :authenticate_user!
  end

  def authenticate_user!
    token = request.headers["Authorization"]&.split(" ")&.last
    payload = JWT.decode(token, SECRET_KEY, true, algorithm: "HS256").first
    @current_user = Users::UserRepository.new.find_kept!(payload["user_id"])
  rescue JWT::DecodeError, ActiveRecord::RecordNotFound
    render json: { error: "Unauthorized" }, status: :unauthorized
  end

  # 任意認証: token があれば current_user を立てる。無効/不在でも 401 にせず nil のまま続行。
  def current_user_if_present
    token = request.headers["Authorization"]&.split(" ")&.last
    return if token.blank?

    payload = JWT.decode(token, SECRET_KEY, true, algorithm: "HS256").first
    @current_user = Users::UserRepository.new.find_kept!(payload["user_id"])
  rescue JWT::DecodeError, ActiveRecord::RecordNotFound
    @current_user = nil
  end

  def current_user
    @current_user
  end
end
