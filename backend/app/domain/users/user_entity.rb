module Users
  class UserEntity
    attr_reader :id

    def self.from_record(record)
      return nil if record.nil?

      new(id: record.id, admin: record.admin)
    end

    def initialize(id:, admin:)
      @id    = id
      @admin = admin
      freeze
    end

    def admin? = @admin == true
    def can_moderate_shops? = admin?
    def manages?(target_user_id) = !@id.nil? && @id == target_user_id
  end
end
