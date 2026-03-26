require "rails_helper"

RSpec.describe Rating do
  describe "#initialize" do
    it "accepts valid values 1 to 5" do
      (1..5).each do |v|
        expect { Rating.new(v) }.not_to raise_error
      end
    end

    it "raises ArgumentError for value 0" do
      expect { Rating.new(0) }.to raise_error(ArgumentError)
    end

    it "raises ArgumentError for value 6" do
      expect { Rating.new(6) }.to raise_error(ArgumentError)
    end

    it "is frozen after initialization" do
      expect(Rating.new(3)).to be_frozen
    end
  end

  describe "#excellent?" do
    it "returns true for 4" do
      expect(Rating.new(4).excellent?).to be true
    end

    it "returns true for 5" do
      expect(Rating.new(5).excellent?).to be true
    end

    it "returns false for 3" do
      expect(Rating.new(3).excellent?).to be false
    end
  end

  describe "#poor?" do
    it "returns true for 1" do
      expect(Rating.new(1).poor?).to be true
    end

    it "returns true for 2" do
      expect(Rating.new(2).poor?).to be true
    end

    it "returns false for 3" do
      expect(Rating.new(3).poor?).to be false
    end
  end

  describe "#==" do
    it "is equal to another Rating with the same value" do
      expect(Rating.new(4)).to eq(Rating.new(4))
    end

    it "is not equal to a Rating with a different value" do
      expect(Rating.new(4)).not_to eq(Rating.new(3))
    end
  end

  describe "#to_f" do
    it "returns the value as Float" do
      expect(Rating.new(3).to_f).to eq(3.0)
    end
  end
end
