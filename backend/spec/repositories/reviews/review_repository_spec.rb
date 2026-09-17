require "rails_helper"

RSpec.describe Reviews::ReviewRepository do
  subject(:repository) { described_class.new }

  describe "#find_kept!" do
    it "returns a kept review" do
      review = create(:review)

      expect(repository.find_kept!(review.id)).to eq(review)
    end

    it "raises ActiveRecord::RecordNotFound for a discarded review" do
      review = create(:review)
      review.discard

      expect { repository.find_kept!(review.id) }.to raise_error(ActiveRecord::RecordNotFound)
    end
  end

  describe "#update!" do
    it "updates the review and returns it" do
      review = create(:review, rating: 3, comment: "Before")

      result = repository.update!(review, rating: 5, comment: "After")

      expect(result).to eq(review)
      expect(review.reload.rating).to eq(5)
      expect(review.comment).to eq("After")
    end
  end

  describe "#discard!" do
    it "soft-deletes the review" do
      review = create(:review)

      repository.discard!(review)

      expect(review.reload.discarded_at).not_to be_nil
    end
  end
end
