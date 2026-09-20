class ShopPolicy < ApplicationPolicy
  def create? = user.present?
  def update? = moderator?
  def approve? = moderator?
  def reject?  = moderator?
  def moderate_index? = moderator?

  def review?
    Shops::ShopEntity.from_record(record).reviewable_by?(Users::UserEntity.from_record(user))
  end

  private

  def moderator?
    (Users::UserEntity.from_record(user)&.can_moderate_shops?) || false
  end
end
