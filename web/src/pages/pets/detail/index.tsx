import { Button, Text, View } from '@tarojs/components'
import Taro, { useDidShow, useRouter } from '@tarojs/taro'
import { useState } from 'react'
import PageBackground from '../../../components/page-background'
import { deletePet, getPet } from '../../../services/pet'
import { navigateBack, openPetEdit } from '../../../utils/navigation'

export default function PetDetail() {
  const { params } = useRouter<{ petId?: string }>()
  const petID = params.petId
  const [name, setName] = useState('')
  const [loading, setLoading] = useState(false)

  async function load() {
    if (!petID) {
      return
    }
    setLoading(true)
    try {
      const pet = await getPet(petID)
      setName(pet.name)
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '加载宠物失败'
      await Taro.showToast({ title: message, icon: 'none' })
    }
    finally {
      setLoading(false)
    }
  }

  async function handleDelete() {
    if (!petID) {
      return
    }
    const result = await Taro.showModal({ title: '删除宠物', content: '确认删除这只宠物吗？' })
    if (!result.confirm) {
      return
    }
    setLoading(true)
    try {
      await deletePet(petID)
      await navigateBack()
    }
    catch (error) {
      const message = error instanceof Error ? error.message : '删除失败，请重试'
      await Taro.showToast({ title: message, icon: 'none' })
      setLoading(false)
    }
  }

  useDidShow(() => {
    void load()
  })

  return (
    <View className="themed-page">
      <PageBackground />
      <Text>{name || '宠物详情'}</Text>
      <Button loading={loading} disabled={loading || !petID} onClick={() => petID && openPetEdit(petID)}>修改名称</Button>
      <Button disabled={loading || !petID} onClick={handleDelete}>删除宠物</Button>
    </View>
  )
}
