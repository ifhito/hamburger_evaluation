class ShopSerializer
  def initialize(shop)
    @shop = shop
  end

  def as_json
    {
      id:              @shop.id,
      name:            @shop.name,
      status:          @shop.status,
      moderation_note: @shop.moderation_note,
      creator:         creator_json
    }
  end

  private

  def creator_json
    return nil unless @shop.creator

    { id: @shop.creator.id, username: @shop.creator.username }
  end
end
