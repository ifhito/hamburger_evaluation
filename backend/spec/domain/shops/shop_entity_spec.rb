require "rails_helper"

RSpec.describe Shops::ShopEntity do
  def entity(status:, creator_id: 100)
    described_class.new(id: 1, status: status, creator_id: creator_id)
  end

  let(:admin)   { Users::UserEntity.new(id: 999, admin: true) }
  let(:owner)   { Users::UserEntity.new(id: 100, admin: false) }
  let(:other)   { Users::UserEntity.new(id: 200, admin: false) }

  describe ".from_record" do
    it "record の属性からエンティティを構築する" do
      record = Struct.new(:id, :status, :creator_id).new(5, "active", 100)
      built = described_class.from_record(record)
      expect(built.viewable_by?(nil)).to be true
    end
  end

  describe "#viewable_by?" do
    context "active" do
      it "匿名でも見える" do
        expect(entity(status: "active").viewable_by?(nil)).to be true
      end
    end

    context "pending" do
      it "匿名は見えない" do
        expect(entity(status: "pending").viewable_by?(nil)).to be false
      end

      it "作成者は見える" do
        expect(entity(status: "pending").viewable_by?(owner)).to be true
      end

      it "他人は見えない" do
        expect(entity(status: "pending").viewable_by?(other)).to be false
      end

      it "admin は見える" do
        expect(entity(status: "pending").viewable_by?(admin)).to be true
      end
    end

    context "rejected" do
      it "作成者は見える(却下理由確認のため)" do
        expect(entity(status: "rejected").viewable_by?(owner)).to be true
      end

      it "admin は見える" do
        expect(entity(status: "rejected").viewable_by?(admin)).to be true
      end

      it "他人/匿名は見えない" do
        expect(entity(status: "rejected").viewable_by?(other)).to be false
        expect(entity(status: "rejected").viewable_by?(nil)).to be false
      end
    end
  end

  describe "#reviewable_by?" do
    it "active はログインユーザーなら誰でも可" do
      expect(entity(status: "active").reviewable_by?(other)).to be true
    end

    it "pending は作成者と admin のみ可" do
      expect(entity(status: "pending").reviewable_by?(owner)).to be true
      expect(entity(status: "pending").reviewable_by?(admin)).to be true
      expect(entity(status: "pending").reviewable_by?(other)).to be false
    end

    it "rejected は誰も不可(作成者・admin も含む)" do
      expect(entity(status: "rejected").reviewable_by?(owner)).to be false
      expect(entity(status: "rejected").reviewable_by?(admin)).to be false
    end

    it "匿名は不可" do
      expect(entity(status: "active").reviewable_by?(nil)).to be true
      expect(entity(status: "pending").reviewable_by?(nil)).to be false
    end
  end

  describe "遷移" do
    it "#approve は active(ShopStatus)" do
      expect(entity(status: "pending").approve).to eq(Shops::ShopStatus.new("active"))
    end

    it "#reject は rejected(ShopStatus)" do
      expect(entity(status: "pending").reject).to eq(Shops::ShopStatus.new("rejected"))
    end
  end
end
