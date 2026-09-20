module Reviews
  class UpdateParameter < Dry::Struct
    module Types
      include Dry.Types()
    end

    attribute? :rating,  Types::Coercible::Integer
    attribute? :comment, Types::String
  end
end
