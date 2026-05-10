class ApplicationPolicy
  attr_reader :user, :record

  def initialize(user, record)
    @user   = user
    @record = record
  end

  def index?   = false
  def show?    = false
  def create?  = true
  def new?     = create?
  def update?  = false
  def edit?    = update?
  def destroy? = false
end
