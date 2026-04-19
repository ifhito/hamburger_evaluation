class ReviewsController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index, :show ]

  def index
    search_param = Reviews::SearchParameter.new(
      rating:  params[:rating],
      keyword: params[:keyword],
      shop_id: params[:shop_id]
    )
    reviews = Reviews::ReviewFinder.new(search_param).search
    render json: reviews.map { |r| ReviewSerializer.new(r).as_json }
  end

  def show
    review = Review.kept.includes(:user, :burger, burger: :burger_stat).find(params[:id])
    render json: ReviewSerializer.new(review).as_json
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  end

  def create
    authorize Review, :create?
    param = Reviews::CreateParameter.new(
      rating:      review_params[:rating],
      comment:     review_params[:comment],
      shop_id:     review_params[:shop_id],
      burger_name: review_params[:burger_name]
    )
    review = Reviews::CreateReviewService.new(user: current_user, params: param).invoke
    render json: ReviewSerializer.new(review).as_json, status: :created
  rescue ActiveRecord::RecordNotFound => e
    render json: { error: e.message }, status: :not_found
  rescue ActiveRecord::RecordInvalid => e
    render json: { errors: e.record.errors.full_messages }, status: :unprocessable_entity
  end

  def update
    review = Review.kept.find(params[:id])
    authorize review
    param  = Reviews::UpdateParameter.new(review_params.to_h.slice("rating", "comment").symbolize_keys)
    review = Reviews::UpdateReviewService.new(review: review, params: param).invoke
    render json: ReviewSerializer.new(review).as_json
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  rescue ActiveRecord::RecordInvalid => e
    render json: { errors: e.record.errors.full_messages }, status: :unprocessable_entity
  end

  def destroy
    review = Review.kept.find(params[:id])
    authorize review
    Reviews::DeleteReviewService.new(review: review).invoke
    head :no_content
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  end

  private

  def review_params
    params.require(:review).permit(:rating, :comment, :shop_id, :burger_name)
  end
end
