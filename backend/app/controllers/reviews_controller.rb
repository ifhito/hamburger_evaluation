class ReviewsController < ApplicationController
  include Authenticatable

  skip_before_action :authenticate_user!, only: [ :index, :show ]

  def index
    reviews = ReviewQuery.call(params)
    render json: reviews.map { |r| ReviewSerializer.new(r).as_json }
  end

  def show
    review = Review.kept.includes(:user, :burger, burger: :burger_stat).find(params[:id])
    render json: ReviewSerializer.new(review).as_json
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  end

  def create
    shop = Shop.find(params[:review][:shop_id])
    burger = find_or_create_burger(shop, params[:review][:burger_name])
    review = current_user.reviews.build(rating: review_params[:rating], comment: review_params[:comment], burger: burger)
    if review.save
      review.burger.reload
      render json: ReviewSerializer.new(review).as_json, status: :created
    else
      render json: { errors: review.errors.full_messages }, status: :unprocessable_entity
    end
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Shop not found" }, status: :not_found
  end

  def update
    review = current_user.reviews.kept.find(params[:id])
    if review.update(review_params)
      render json: ReviewSerializer.new(review).as_json
    else
      render json: { errors: review.errors.full_messages }, status: :unprocessable_entity
    end
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  end

  def destroy
    review = current_user.reviews.kept.find(params[:id])
    review.discard
    head :no_content
  rescue ActiveRecord::RecordNotFound
    render json: { error: "Review not found" }, status: :not_found
  end

  private

  def find_or_create_burger(shop, burger_name)
    burger = shop.burgers.find_by(name: burger_name)
    unless burger
      burger = Burger.create!(name: burger_name)
      ShopsAndBurger.create!(shop: shop, burger: burger)
    end
    burger
  end

  def review_params
    params.require(:review).permit(:rating, :comment)
  end
end
