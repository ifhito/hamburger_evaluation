class ReviewPolicy < ApplicationPolicy
  def update?  = entity.editable_by?(viewer)
  def destroy? = entity.deletable_by?(viewer)

  private

  def entity = Reviews::ReviewEntity.from_record(record)
  def viewer = Users::UserEntity.from_record(user)
end
