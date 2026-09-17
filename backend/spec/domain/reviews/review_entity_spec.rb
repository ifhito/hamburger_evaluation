require "rails_helper"

RSpec.describe Reviews::ReviewEntity do
  let(:author) { Users::UserEntity.new(id: 10, admin: false) }
  let(:other)  { Users::UserEntity.new(id: 20, admin: false) }

  def entity(author_id: 10)
    described_class.new(id: 1, author_id: author_id, rating: 4, comment: "good")
  end

  describe ".from_record" do
    it "record の属性からエンティティを構築する" do
      record = Struct.new(:id, :user_id, :rating, :comment).new(3, 10, 5, "nice")
      built = described_class.from_record(record)
      expect(built.author_id).to eq(10)
      expect(built.editable_by?(author)).to be true
    end
  end

  describe "#editable_by? / #deletable_by?" do
    it "作成者本人は編集・削除できる" do
      expect(entity.editable_by?(author)).to be true
      expect(entity.deletable_by?(author)).to be true
    end

    it "他人は編集・削除できない" do
      expect(entity.editable_by?(other)).to be false
      expect(entity.deletable_by?(other)).to be false
    end

    it "匿名(nil)は編集・削除できない" do
      expect(entity.editable_by?(nil)).to be false
      expect(entity.deletable_by?(nil)).to be false
    end
  end

  it "初期化後は frozen" do
    expect(entity).to be_frozen
  end
end
