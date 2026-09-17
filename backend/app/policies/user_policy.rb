class UserPolicy < ApplicationPolicy
  def update?  = (Users::UserEntity.from_record(user)&.manages?(record.id)) || false
  def destroy? = update?
end
