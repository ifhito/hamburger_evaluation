class UserPolicy < ApplicationPolicy
  def update?  = record == user
  def destroy? = record == user
end
