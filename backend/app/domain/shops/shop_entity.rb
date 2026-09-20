module Shops
  class ShopEntity
    attr_reader :id

    def self.from_record(record)
      new(id: record.id, status: record.status, creator_id: record.creator_id)
    end

    def initialize(id:, status:, creator_id:)
      @id         = id
      @status     = Shops::ShopStatus.new(status)
      @creator_id = creator_id
    end

    # viewer: Users::UserEntity | nil (匿名)
    def viewable_by?(viewer)
      return true  if @status.active?
      return false if viewer.nil?

      viewer.admin? || owner?(viewer.id)
    end

    # viewer: Users::UserEntity | nil (匿名)
    def reviewable_by?(viewer)
      return false if @status.rejected?
      return true  if @status.active?
      return false if viewer.nil?

      viewer.admin? || owner?(viewer.id)
    end

    def approve = @status.approve
    def reject  = @status.reject

    private

    def owner?(viewer_id)
      !@creator_id.nil? && @creator_id == viewer_id
    end
  end
end
