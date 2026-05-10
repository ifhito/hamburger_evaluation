require "rails_helper"

RSpec.describe Review, type: :model do
  describe "associations" do
    it { is_expected.to belong_to(:user) }
    it { is_expected.to belong_to(:burger) }
  end

  describe "soft delete (discard)" do
    it "sets discarded_at when discarded" do
      review = create(:review)
      expect { review.discard }.to change { review.discarded_at }.from(nil)
    end

    it "excludes discarded reviews from Review.kept" do
      review = create(:review)
      review.discard
      expect(Review.kept).not_to include(review)
    end

    it "includes active reviews in Review.kept" do
      review = create(:review)
      expect(Review.kept).to include(review)
    end
  end

  describe "validations" do
    it "is invalid without a rating" do
      review = build(:review, rating: nil)
      expect(review).not_to be_valid
    end

    it "is invalid without a comment" do
      review = build(:review, comment: nil)
      expect(review).not_to be_valid
    end
  end

  describe "named scopes" do
    describe ".by_rating" do
      it "returns only reviews with the specified rating" do
        create(:review, rating: 3)
        r5 = create(:review, rating: 5)
        expect(Review.by_rating(5)).to contain_exactly(r5)
      end
    end

    describe ".keyword_search" do
      it "returns reviews whose comment contains the keyword" do
        matched = create(:review, comment: "外はcrispy中はジューシー")
        create(:review, comment: "普通のバーガー")
        expect(Review.keyword_search("crispy")).to contain_exactly(matched)
      end

      it "is case-insensitive (ILIKE behavior)" do
        upper = create(:review, comment: "CRISPY texture")
        lower = create(:review, comment: "crispy texture")
        expect(Review.keyword_search("crispy")).to contain_exactly(upper, lower)
      end
    end

    describe ".recent" do
      it "returns reviews in descending order of created_at" do
        old_review  = create(:review, created_at: 2.days.ago)
        new_review  = create(:review, created_at: 1.day.ago)
        result = Review.recent
        expect(result.first).to eq(new_review)
        expect(result.last).to eq(old_review)
      end
    end
  end

  describe "callbacks" do
    describe "after_create_commit" do
      it "publishes a review-changed event with the burger_id after commit" do
        burger = create(:burger)

        expect(Reviews::ReviewEvents).to receive(:review_changed_for_burger).with(burger.id)

        create(:review, burger: burger)
      end
    end

    describe "after_update_commit" do
      it "publishes a review-changed event when rating changes" do
        review = create(:review, rating: 3)

        expect(Reviews::ReviewEvents).to receive(:review_changed_for_burger).with(review.burger_id)

        review.update!(rating: 5)
      end

      it "does not publish a review-changed event when only comment changes" do
        review = create(:review, rating: 3, comment: "original")

        expect(Reviews::ReviewEvents).not_to receive(:review_changed_for_burger)

        review.update!(comment: "updated")
      end
    end

    describe "after_discard" do
      it "publishes a review-changed event when discarded" do
        review = create(:review)

        expect(Reviews::ReviewEvents).to receive(:review_changed_for_burger).with(review.burger_id)

        review.discard
      end
    end
  end
end
