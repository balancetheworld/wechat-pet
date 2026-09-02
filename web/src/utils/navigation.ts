import Taro from '@tarojs/taro'
import { routes, withQuery } from '../constants/routes'

type TabRoute = (typeof routes.tabs)[keyof typeof routes.tabs]

export function navigateTo(url: string) {
  return Taro.navigateTo({ url })
}

export function switchTab(url: TabRoute) {
  return Taro.switchTab({ url })
}

export function reLaunch(url: string) {
  return Taro.reLaunch({ url })
}

export function navigateBack(delta = 1) {
  return Taro.navigateBack({ delta })
}

export function openPetDetail(petId: string) {
  return navigateTo(withQuery(routes.pages.petDetail, { petId }))
}

export function openPetEdit(petId: string) {
  return navigateTo(withQuery(routes.pages.petEdit, { petId }))
}

export function openProfileEdit() {
  return navigateTo(routes.pages.profileEdit)
}

export function openShare(shareId: string) {
  return navigateTo(withQuery(routes.pages.share, { shareId }))
}
