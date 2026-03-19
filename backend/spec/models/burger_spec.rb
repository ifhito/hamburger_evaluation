require "rails_helper"

RSpec.describe BurgerStat, type: :model do
  describe ".recalculate_for" do
    let(:burger) { create(:burger) }

    context "when there are active reviews" do
      before do
        create(:review, burger: burger, rating: 4)
        create(:review, burger: burger, rating: 5)
      end

      it "stores the correct average_rating" do
        BurgerStat.recalculate_for(burger)
        stat = BurgerStat.find_by(burger: burger)
        expect(stat.average_rating).to eq(4.5)
      end

      it "stores the correct review_count" do
        BurgerStat.recalculate_for(burger)
        stat = BurgerStat.find_by(burger: burger)
        expect(stat.review_count).to eq(2)
      end
    end

    context "when some reviews are discarded" do
      before do
        create(:review, burger: burger, rating: 5)
        discarded = create(:review, burger: burger, rating: 1)
        discarded.update_column(:discarded_at, Time.current)
      end

      it "excludes discarded reviews from the calculation" do
        BurgerStat.recalculate_for(burger)
        stat = BurgerStat.find_by(burger: burger)
        expect(stat.average_rating).to eq(5.0)
        expect(stat.review_count).to eq(1)
      end
    end

    context "when there are no active reviews" do
      it "sets average_rating to 0.0 and review_count to 0" do
        BurgerStat.recalculate_for(burger)
        stat = BurgerStat.find_by(burger: burger)
        expect(stat.average_rating).to eq(0.0)
        expect(stat.review_count).to eq(0)
      end
    end

    context "when called multiple times" do
      it "updates the existing record instead of creating a new one" do
        BurgerStat.recalculate_for(burger)
        create(:review, burger: burger, rating: 3)
        BurgerStat.recalculate_for(burger)
        expect(BurgerStat.where(burger: burger).count).to eq(1)
      end
    end
  end
end
