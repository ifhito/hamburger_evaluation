module Reviews
  class ReviewEntity
    attr_reader :id, :author_id

    def self.from_record(record)
      new(id: record.id, author_id: record.user_id, rating: record.rating, comment: record.comment)
    end

    def initialize(id:, author_id:, rating:, comment:)
      @id        = id
      @author_id = author_id
      @rating    = Reviews::Rating.new(rating)
      @comment   = comment
      freeze
    end

    # viewer: Users::UserEntity | nil
    def editable_by?(viewer)
      !viewer.nil? && !@author_id.nil? && @author_id == viewer.id
    end

    def deletable_by?(viewer)
      editable_by?(viewer)
    end
  end
end
