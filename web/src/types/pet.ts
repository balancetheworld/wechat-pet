export interface Pet {
  id: string
  name: string
}

export interface PetProfile extends Pet {
  avatar_asset_id: string
  cover_asset_id: string
  breed: string
  gender: string
  sterilized: boolean
  birthday?: string
  home_date?: string
  age: number
  companion_days: number
  next_birthday_days?: number
}

export interface CreatePetRequest {
  name: string
  avatar_asset_id?: string
  breed?: string
  gender?: string
  sterilized?: boolean
  birthday?: string
  home_date?: string
}

export interface UpdatePetRequest {
  name: string
  breed: string
  gender: string
  sterilized: boolean
  birthday: string
  home_date: string
}
