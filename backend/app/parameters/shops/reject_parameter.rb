module Shops
  class RejectParameter < Dry::Struct
    module Types
      include Dry.Types()
    end

    attribute? :moderation_note, Types::String.optional
  end
end
