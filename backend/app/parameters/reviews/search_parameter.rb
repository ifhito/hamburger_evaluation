module Reviews
  class SearchParameter < Dry::Struct
    module Types
      include Dry.Types()
    end

    attribute? :rating,  Types::Coercible::Integer.optional
    attribute? :keyword, Types::String.optional
    attribute? :shop_id, Types::Coercible::Integer.optional
  end
end
