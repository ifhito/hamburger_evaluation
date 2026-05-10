class ApplicationController < ActionController::API
  include Pundit::Authorization

  rescue_from Pundit::NotAuthorizedError, with: :render_forbidden
  rescue_from Dry::Struct::Error, with: :render_invalid_params

  private

  def render_forbidden
    render json: { error: "Forbidden" }, status: :forbidden
  end

  def render_invalid_params(e)
    render json: { error: e.message }, status: :unprocessable_entity
  end
end
